import { useCallback, useEffect, useState, type MouseEvent, type ReactNode } from "react";
import { type Card, type Session, type Workspace, post, useData } from "./api";
import { Detail, DetailContext } from "./Detail";
import { Logo } from "./Logo";
import { FloorPage } from "./pages/Floor";
import { NowPage } from "./pages/Now";
import { ProjectsPage } from "./pages/Projects";
import { ReviewPage } from "./pages/Review";
import { Queues } from "./pages/Queues";
import { Triage } from "./pages/Triage";
import { TradeoffPage } from "./pages/Tradeoff";

// "Now" and "Floor" are for everyone. The rest are the tools for shaping the line.
const PAGES = [
  { id: "now", label: "Now" },
  { id: "floor", label: "Floor" },
  { id: "queues", label: "Queues" },
  { id: "projects", label: "Projects" },
  { id: "triage", label: "Triage" },
  { id: "tradeoff", label: "Trade-off" },
  { id: "review", label: "Review" },
];

function usePath(): [string, (to: string) => void] {
  const [path, setPath] = useState(window.location.pathname + window.location.search);
  useEffect(() => {
    const onPop = () => setPath(window.location.pathname + window.location.search);
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  const go = (to: string) => {
    window.history.pushState(null, "", to);
    setPath(to);
    window.scrollTo(0, 0);
  };
  return [path, go];
}

export type Go = (to: string) => void;

export function Link(props: { to: string; go: Go; className?: string; children: ReactNode }) {
  const onClick = (e: MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    props.go(props.to);
  };
  return (
    <a href={props.to} onClick={onClick} className={props.className}>
      {props.children}
    </a>
  );
}

function themeCss(theme: Session["theme"]) {
  const vars = (t: Record<string, string>) =>
    Object.entries(t)
      .map(([k, v]) => `--${k}:${v};`)
      .join("");
  return `:root{${vars(theme.light)}}@media (prefers-color-scheme: dark){:root{${vars(theme.dark)}}}`;
}

export function App() {
  const { data: session, error } = useData<Session>("/api/session");
  const [path, go] = usePath();
  const [detail, setDetail] = useState<Card | null>(null);
  const close = useCallback(() => setDetail(null), []);
  const [round, setRound] = useState(0); // bumping it reloads the page in view

  useEffect(() => {
    if (session) document.title = session.brand.name;
  }, [session]);
  useEffect(() => setDetail(null), [path]);

  if (error) return <p className="notice stop">{error}</p>;
  if (!session) return null;
  if (session.workspaces.length === 0)
    return <p className="notice">You are signed in as {session.user}, but have not been added to a workspace yet.</p>;

  const [, w, slug, page] = path.split("?")[0].split("/");
  const ws = (w === "w" && session.workspaces.find((x) => x.slug === slug)) || null;
  if (!ws || !PAGES.some((p) => p.id === page)) {
    // People the app recognises land on their own work; everyone else on the floor.
    const to = ws ?? session.workspaces[0];
    const fallback = `/w/${to.slug}/${to.me ? "now" : "floor"}`;
    if (path !== fallback) queueMicrotask(() => go(fallback));
    return null;
  }

  return (
    <DetailContext.Provider value={setDetail}>
      <style>{themeCss(session.theme)}</style>
      <header className="top">
        <Link to={`/w/${ws.slug}/floor`} go={go} className="brand">
          {session.brand.logo ? <img className="logo" src={session.brand.logo} alt="" /> : <Logo />}
          {session.brand.name}
        </Link>
        {session.workspaces.length > 1 ? (
          <select className="ws-pick" aria-label="Workspace" value={ws.slug} onChange={(e) => go(`/w/${e.target.value}/${page}`)}>
            {session.workspaces.map((x) => (
              <option key={x.slug} value={x.slug}>
                {x.name}
              </option>
            ))}
          </select>
        ) : (
          <span className="ws-name">{ws.name}</span>
        )}
        <nav>
          {PAGES.map((p) => (
            <Link key={p.id} to={`/w/${ws.slug}/${p.id}`} go={go} className={p.id === page ? "on" : undefined}>
              {p.label}
            </Link>
          ))}
        </nav>
        {ws.canTriage && (
          <button
            type="button"
            className="check"
            title="Read the work items again now, without waiting for the next refresh"
            onClick={async () => {
              await post(`/api/w/${ws.slug}/refresh`, {}).catch(() => {});
              setRound((n) => n + 1);
            }}
          >
            Check now
          </button>
        )}
        <span className="who">
          {session.user}
          {!ws.canWrite && " · read-only"}
        </span>
      </header>
      <main key={`${ws.slug}/${round}`}>
        <Page page={page} ws={ws} session={session} go={go} />
      </main>
      {detail && <Detail card={detail} close={close} />}
    </DetailContext.Provider>
  );
}

function Page(props: { page: string; ws: Workspace; session: Session; go: Go }) {
  switch (props.page) {
    case "now":
      return <NowPage {...props} />;
    case "triage":
      return <Triage {...props} />;
    case "queues":
      return <Queues {...props} />;
    case "projects":
      return <ProjectsPage {...props} />;
    case "tradeoff":
      return <TradeoffPage {...props} />;
    case "review":
      return <ReviewPage {...props} />;
    default:
      return <FloorPage {...props} />;
  }
}
