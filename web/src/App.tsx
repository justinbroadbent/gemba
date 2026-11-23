import { useEffect, useState, type MouseEvent, type ReactNode } from "react";
import { type Session, type Workspace, useData } from "./api";
import { Board } from "./pages/Board";
import { Queues } from "./pages/Queues";
import { Triage } from "./pages/Triage";
import { TradeoffPage } from "./pages/Tradeoff";

const PAGES = [
  { id: "board", label: "Board", kanji: "看板" },
  { id: "triage", label: "Triage", kanji: "受付" },
  { id: "queues", label: "Queues", kanji: "行列" },
  { id: "tradeoff", label: "Trade-off", kanji: "決断" },
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

  useEffect(() => {
    if (session) document.title = session.brand.name;
  }, [session]);

  if (error) return <p className="notice stop">{error}</p>;
  if (!session) return null;
  if (session.workspaces.length === 0)
    return <p className="notice">You are signed in as {session.user}, but have not been added to a workspace yet.</p>;

  const [, w, slug, page = "board"] = path.split("?")[0].split("/");
  const ws = (w === "w" && session.workspaces.find((x) => x.slug === slug)) || null;
  if (!ws || !PAGES.some((p) => p.id === page)) {
    const fallback = `/w/${session.workspaces[0].slug}/board`;
    if (path !== fallback) queueMicrotask(() => go(fallback));
    return null;
  }

  return (
    <>
      <style>{themeCss(session.theme)}</style>
      <header className="top">
        <Link to={`/w/${ws.slug}/board`} go={go} className="brand">
          <span className="seal" aria-hidden="true">
            {session.brand.mark}
          </span>
          {session.brand.name}
        </Link>
        {session.workspaces.length > 1 ? (
          <select
            className="ws-pick"
            aria-label="Workspace"
            value={ws.slug}
            onChange={(e) => go(`/w/${e.target.value}/${page}`)}
          >
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
              <small aria-hidden="true">{p.kanji}</small>
            </Link>
          ))}
        </nav>
        <span className="who">
          {session.user}
          {!ws.canWrite && " · read-only"}
        </span>
      </header>
      <main key={ws.slug}>
        <Page page={page} ws={ws} session={session} go={go} />
      </main>
    </>
  );
}

function Page(props: { page: string; ws: Workspace; session: Session; go: Go }) {
  switch (props.page) {
    case "triage":
      return <Triage {...props} />;
    case "queues":
      return <Queues {...props} />;
    case "tradeoff":
      return <TradeoffPage {...props} />;
    default:
      return <Board {...props} />;
  }
}
