import { useState } from "react";
import { type Note, type Preview, type Session, type Tradeoff, type Workspace, ApiError, post, useData } from "../api";
import { type Go } from "../App";
import { CardView } from "../CardView";

export function TradeoffPage({ ws, session, go }: { ws: Workspace; session: Session; go: Go }) {
  const params = new URLSearchParams(window.location.search);
  const item = params.get("item") ?? "";
  const cell = params.get("cell") ?? "";
  const query = item && cell ? `?item=${item}&cell=${encodeURIComponent(cell)}` : "";
  const { data, error } = useData<Tradeoff>(`/api/w/${ws.slug}/tradeoff${query}`);
  const [written, setWritten] = useState<Note[] | null>(null);

  const choose = (nextItem: string, nextCell: string) => {
    const picked = data?.candidates.find((c) => String(c.id) === nextItem);
    const useCell = nextCell || picked?.cell || "";
    const q = new URLSearchParams();
    if (nextItem) q.set("item", nextItem);
    if (useCell) q.set("cell", useCell);
    go(`/w/${ws.slug}/tradeoff${q.size ? `?${q}` : ""}`);
  };

  if (written)
    return (
      <>
        <header className="page-head">
          <h1>Decision recorded</h1>
          <p>These notes were written on the affected items, where the team will see them.</p>
        </header>
        {written.map((n) => (
          <article key={n.id} className="card note">
            <p className="card-meta">
              <span className="num">#{n.id}</span>
              <span>{n.title}</span>
            </p>
            <p className="card-title">{n.text}</p>
          </article>
        ))}
        <p>
          <button
            type="button"
            onClick={() => {
              setWritten(null);
              go(`/w/${ws.slug}/queues`);
            }}
          >
            See the queues
          </button>
        </p>
      </>
    );

  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;

  return (
    <>
      <header className="page-head">
        <h1>Trade-off</h1>
        <p>Something new has become the priority. See what it displaces before deciding.</p>
      </header>
      <div className="pickers">
        <label>
          What is the new priority?
          <select value={item} onChange={(e) => choose(e.target.value, "")}>
            <option value="">Choose an item</option>
            {data.candidates.map((c) => (
              <option key={c.id} value={c.id}>
                #{c.id} {c.title}
                {c.stage === "intake" ? " (not yet queued)" : ""}
              </option>
            ))}
          </select>
        </label>
        <label>
          Which cell does the work?
          <select value={cell} onChange={(e) => choose(item, e.target.value)} disabled={!item}>
            <option value="">Choose a cell</option>
            {ws.cells.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
      </div>
      {data.preview && (
        <Decision
          key={`${item}/${cell}`}
          preview={data.preview}
          ws={ws}
          defaultAuthorizer={session.user === "local" ? "" : session.user}
          onDone={setWritten}
        />
      )}
    </>
  );
}

function Decision(props: {
  preview: Preview;
  ws: Workspace;
  defaultAuthorizer: string;
  onDone: (notes: Note[]) => void;
}) {
  const { preview: p, ws } = props;
  const [pause, setPause] = useState<number | null>(null);
  const [by, setBy] = useState(props.defaultAuthorizer);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const cellName = p.lane.cell.name;
  const weeks = ws.horizonWeeks;

  let cost: string;
  if (p.alreadyAbove) cost = `It is already within the ${weeks}-week line. Moving it to the front displaces nothing.`;
  else if (p.falls) cost = `It pushes one item past the ${weeks}-week line:`;
  else if (!p.lane.measured) cost = `${cellName} has no finished work to measure pace from yet, so there is no line to cross.`;
  else if (p.lane.line === 0)
    cost = `Work already in progress fills ${cellName}'s next ${weeks} weeks. At the front of the queue it still waits; to start it sooner, pause something below.`;
  else cost = `${cellName} has room within the ${weeks}-week line. Nothing is displaced.`;

  const submit = async () => {
    setBusy(true);
    setProblem(null);
    try {
      const res = await post<{ written: Note[] }>(`/api/w/${ws.slug}/tradeoff`, {
        item: p.item.id,
        cell: p.lane.cell.id,
        pause,
        authorizedBy: by,
      });
      props.onDone(res.written);
    } catch (e) {
      setProblem(e instanceof ApiError ? e.message : "Could not record the decision.");
      setBusy(false);
    }
  };

  return (
    <form
      className="decision"
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <section>
        <h2>The new priority</h2>
        <CardView plain card={p.item} />
      </section>

      <section>
        <h2>Put it at the front of the {cellName} queue</h2>
        <p>{cost}</p>
        {p.falls && <CardView plain card={p.falls} owner={false} />}
      </section>

      <section>
        <h2>Or start it now by pausing work in progress</h2>
        {p.lane.doing.length === 0 ? (
          <p>Nothing is in progress in {cellName}, so it can be started without pausing anything.</p>
        ) : (
          <div className="choices">
            <label className="choice">
              <input type="radio" name="pause" checked={pause === null} onChange={() => setPause(null)} />
              <span>Do not pause anything</span>
            </label>
            {p.lane.doing.map((c) => (
              <label key={c.id} className="choice">
                <input type="radio" name="pause" checked={pause === c.id} onChange={() => setPause(c.id)} />
                <CardView plain card={c} />
              </label>
            ))}
          </div>
        )}
      </section>

      {ws.canWrite ? (
        <section className="sign">
          <label>
            Authorized by
            <input value={by} onChange={(e) => setBy(e.target.value)} placeholder="Name of who made the call" required />
          </label>
          <button type="submit" disabled={busy}>
            {pause === null ? "Record decision" : "Record decision and pause"}
          </button>
          {problem && <p className="notice stop">{problem}</p>}
        </section>
      ) : (
        <p className="empty">You have read-only access, so you can explore the cost but not record a decision.</p>
      )}
    </form>
  );
}
