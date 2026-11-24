import { useCallback, useEffect, useRef, useState } from "react";
import { type Workspace, get } from "./api";
import { type Go } from "./App";

type Ref = { id: number; title: string };
type Pulse = {
  as: string;
  triage: Ref[];
  doing: Ref[];
  waiting: Ref[];
  next: Ref | null;
  blocked: Ref[];
  risk: Ref[];
  chase: Ref[];
  alerts: Ref[];
};
export type Toast = { key: number; text: string; to?: string };
type Found = Omit<Toast, "key">;

const EVERY = 30_000; // how often to look for changes
const STAYS = 9_000; // how long a toast stays up
const WANTED = "gemba:notify"; // this browser asked for notifications from the operating system

/**
 * What changed between two looks, as short sentences. The list is kept short
 * on purpose: only things the viewer would want to be interrupted for.
 */
function changes(was: Pulse, now: Pulse, slug: string): Found[] {
  const out: Found[] = [];
  const fresh = (a: Ref[], b: Ref[]) => b.filter((x) => !a.some((y) => y.id === x.id));
  const said = new Set<number>(); // one toast per item per look

  const triage = fresh(was.triage, now.triage);
  if (triage.length === 1) out.push({ text: `New to triage: ${triage[0].title}`, to: `/w/${slug}/triage` });
  else if (triage.length > 1) out.push({ text: `${triage.length} new items to triage`, to: `/w/${slug}/triage` });

  if (now.as) {
    const mine = `/w/${slug}/now`;
    const say = (c: Ref, text: string) => {
      if (said.has(c.id)) return;
      said.add(c.id);
      out.push({ text, to: mine });
    };
    for (const c of fresh(was.doing, now.doing)) say(c, `Now in your hands: ${c.title}`);
    for (const c of fresh(was.waiting, now.waiting)) say(c, `Set to waiting: ${c.title}`);
    for (const c of fresh([...now.doing, ...now.waiting], was.doing)) say(c, `No longer in your hands: ${c.title}`);
    // Someone cleared the way. Becoming blocked is not announced: the holder is usually the one who said so.
    for (const c of fresh(now.blocked, was.blocked)) if (now.doing.some((d) => d.id === c.id)) say(c, `No longer blocked: ${c.title}`);
    for (const c of fresh(was.risk, now.risk)) say(c, c.title);
    for (const c of fresh(was.chase, now.chase)) say(c, `Time to chase: ${c.title} is still waiting`);
    if (now.next && now.next.id !== was.next?.id && !now.doing.some((c) => c.id === now.next!.id))
      say(now.next, `Your next item is now: ${now.next.title}`);
  }

  // For managers: something new on the floor needs someone to step in.
  const alerts = fresh(was.alerts, now.alerts).filter((a) => !said.has(a.id));
  if (alerts.length === 1) out.push({ text: alerts[0].title, to: `/w/${slug}/floor` });
  else if (alerts.length > 1) out.push({ text: `${alerts.length} new things need attention on the floor`, to: `/w/${slug}/floor` });
  return out;
}

function recall(key: string) {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

const supported = () => typeof window !== "undefined" && "Notification" in window;
const wanted = () => supported() && Notification.permission === "granted" && recall(WANTED) === "1";

/**
 * Watches for new triage items and for changes to the viewer's own work, and
 * says so in a toast. A change made from this browser is not announced back to
 * the person who made it. When the viewer has asked for it, the same message
 * is also raised through the operating system while they are looking elsewhere.
 */
export function useWatch(ws: Workspace, go: Go, brand: string) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [notifying, setNotifying] = useState(wanted);
  const base = useRef<Pulse | null>(null);
  const serial = useRef(0);

  const push = useCallback(
    (t: Found, system = false) => {
      const key = ++serial.current;
      setToasts((all) => [...all.slice(-3), { ...t, key }]);
      setTimeout(() => setToasts((all) => all.filter((x) => x.key !== key)), STAYS);
      if (system && wanted() && (document.hidden || !document.hasFocus())) {
        try {
          const n = new Notification(brand, { body: t.text, tag: `gemba-${ws.slug}-${key}` });
          n.onclick = () => {
            window.focus();
            if (t.to) go(t.to);
            n.close();
          };
        } catch {
          /* some browsers only allow this from a service worker; the toast is still there */
        }
      }
    },
    [brand, go, ws.slug],
  );
  const dismiss = useCallback((key: number) => setToasts((all) => all.filter((x) => x.key !== key)), []);

  // "quiet" resets the baseline; "auto" announces changes; "asked" also says when there are none.
  const look = useCallback(
    async (mode: "quiet" | "auto" | "asked") => {
      const as = ws.me ?? recall(`gemba:${ws.slug}:me`);
      const now = await get<Pulse>(`/api/w/${ws.slug}/pulse${as ? `?as=${encodeURIComponent(as)}` : ""}`).catch(() => null);
      if (!now) return;
      const was = base.current;
      base.current = now;
      const found = mode === "quiet" || !was || was.as !== now.as ? [] : changes(was, now, ws.slug);
      found.forEach((f) => push(f, mode === "auto"));
      if (mode === "asked" && found.length === 0) push({ text: "Checked just now. Nothing new." });
    },
    [ws, push],
  );

  useEffect(() => {
    base.current = null;
    void look("quiet");
    // Keep looking while the tab is in the background only if that can reach the viewer.
    const timer = setInterval(() => (!document.hidden || wanted()) && void look("auto"), EVERY);
    const own = () => void look("quiet"); // the viewer just changed something themselves
    window.addEventListener("gemba:wrote", own);
    return () => {
      clearInterval(timer);
      window.removeEventListener("gemba:wrote", own);
    };
  }, [look]);

  /** Turns operating-system notifications on (asking the browser first) or off. */
  const toggle = useCallback(async () => {
    if (!supported()) return push({ text: "This browser cannot show notifications." });
    try {
      if (notifying) {
        localStorage.setItem(WANTED, "0");
        setNotifying(false);
        return;
      }
      const answer = Notification.permission === "granted" ? "granted" : await Notification.requestPermission();
      if (answer !== "granted") return push({ text: "Notifications are blocked for this site in the browser's settings." });
      localStorage.setItem(WANTED, "1");
      setNotifying(true);
      push({ text: "Notifications are on. You will be told when you are looking elsewhere." });
    } catch {
      push({ text: "Could not change notifications in this browser." });
    }
  }, [notifying, push]);

  return { toasts, dismiss, look, notifying, toggle, canNotify: supported() };
}

export function Toasts({ toasts, dismiss, go }: { toasts: Toast[]; dismiss: (key: number) => void; go: Go }) {
  return (
    <div className="toasts" role="status" aria-live="polite">
      {toasts.map((t) => (
        <div key={t.key} className="toast">
          {t.to ? (
            <button
              type="button"
              className="toast-text"
              onClick={() => {
                dismiss(t.key);
                go(t.to!);
              }}
            >
              {t.text}
            </button>
          ) : (
            <span className="toast-text">{t.text}</span>
          )}
          <button type="button" className="toast-close" aria-label="Dismiss" onClick={() => dismiss(t.key)}>
            ×
          </button>
        </div>
      ))}
    </div>
  );
}
