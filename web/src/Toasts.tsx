import { useCallback, useEffect, useRef, useState } from "react";
import { type Workspace, get } from "./api";
import { type Go } from "./App";

type Ref = { id: number; title: string };
type Pulse = { as: string; triage: Ref[]; doing: Ref[]; waiting: Ref[]; next: Ref | null };
export type Toast = { key: number; text: string; to?: string };

const EVERY = 30_000; // how often to look for changes
const STAYS = 9_000; // how long a toast stays up

/** What changed between two looks, as short sentences. */
function changes(was: Pulse, now: Pulse, slug: string): Omit<Toast, "key">[] {
  const out: Omit<Toast, "key">[] = [];
  const fresh = (a: Ref[], b: Ref[]) => b.filter((x) => !a.some((y) => y.id === x.id));

  const triage = fresh(was.triage, now.triage);
  if (triage.length === 1) out.push({ text: `New to triage: ${triage[0].title}`, to: `/w/${slug}/triage` });
  else if (triage.length > 1) out.push({ text: `${triage.length} new items to triage`, to: `/w/${slug}/triage` });

  if (now.as) {
    const mine = `/w/${slug}/now`;
    for (const c of fresh(was.doing, now.doing)) out.push({ text: `Now in your hands: ${c.title}`, to: mine });
    for (const c of fresh(was.waiting, now.waiting)) out.push({ text: `Set to waiting: ${c.title}`, to: mine });
    for (const c of fresh([...now.doing, ...now.waiting], was.doing))
      out.push({ text: `No longer in your hands: ${c.title}`, to: mine });
    if (now.next && now.next.id !== was.next?.id && !now.doing.some((c) => c.id === now.next!.id))
      out.push({ text: `Your next item is now: ${now.next.title}`, to: mine });
  }
  return out;
}

function remembered(ws: Workspace) {
  if (ws.me) return ws.me;
  try {
    return localStorage.getItem(`gemba:${ws.slug}:me`) ?? "";
  } catch {
    return "";
  }
}

/**
 * Watches for new triage items and for changes to the viewer's own work, and
 * says so in a toast. A change made from this browser is not announced back to
 * the person who made it.
 */
export function useWatch(ws: Workspace) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const base = useRef<Pulse | null>(null);
  const serial = useRef(0);

  const push = useCallback((t: Omit<Toast, "key">) => {
    const key = ++serial.current;
    setToasts((all) => [...all.slice(-3), { ...t, key }]);
    setTimeout(() => setToasts((all) => all.filter((x) => x.key !== key)), STAYS);
  }, []);
  const dismiss = useCallback((key: number) => setToasts((all) => all.filter((x) => x.key !== key)), []);

  // "quiet" resets the baseline; "auto" announces changes; "asked" also says when there are none.
  const look = useCallback(
    async (mode: "quiet" | "auto" | "asked") => {
      const as = remembered(ws);
      const now = await get<Pulse>(`/api/w/${ws.slug}/pulse${as ? `?as=${encodeURIComponent(as)}` : ""}`).catch(() => null);
      if (!now) return;
      const was = base.current;
      base.current = now;
      const found = mode === "quiet" || !was || was.as !== now.as ? [] : changes(was, now, ws.slug);
      found.forEach(push);
      if (mode === "asked" && found.length === 0) push({ text: "Checked just now. Nothing new." });
    },
    [ws, push],
  );

  useEffect(() => {
    base.current = null;
    void look("quiet");
    const timer = setInterval(() => !document.hidden && void look("auto"), EVERY);
    const own = () => void look("quiet"); // the viewer just changed something themselves
    window.addEventListener("gemba:wrote", own);
    return () => {
      clearInterval(timer);
      window.removeEventListener("gemba:wrote", own);
    };
  }, [look]);

  return { toasts, dismiss, look };
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
