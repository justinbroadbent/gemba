import { useState } from "react";
import { type Candidate, type Note, type Preview, type Session, type Tradeoff, type Workspace, ApiError, post, useData } from "../api";
import { type Go } from "../App";
import { CardView } from "../CardView";
import { TypeIcon } from "../TypeIcon";

const SHOWN = 8; // matches listed before asking for a narrower search

/**
 * The conversation a manager has with their own manager: this new thing is the
 * priority, so what gives? The page shows the cost before anyone decides, and
 * writes the decision on the items it touches.
 */
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
              go(`/w/${ws.slug}/floor`);
            }}
          >
            Back to the floor
          </button>
        </p>
      </>
    );

  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;
  const chosen = data.candidates.find((c) => String(c.id) === item);

  return (
    <>
      <header className="page-head">
        <h1>Trade-off</h1>
        <p>Something new has become the priority. See what it displaces before deciding.</p>
      </header>
      <div className="pickers">
        <Finder candidates={data.candidates} chosen={chosen} pick={(id) => choose(id, "")} />
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

/** Finds the item among hundreds: type part of its title, its number or its project. */
function Finder({ candidates, chosen, pick }: { candidates: Candidate[]; chosen?: Candidate; pick: (id: string) => void }) {
  const [text, setText] = useState("");
  if (chosen)
    return (
      <div className="finder">
        <label>What is the new priority?</label>
        <p className="chosen">
          <TypeIcon icon={chosen.icon} name={chosen.typeName} />
          <span>
            {chosen.title} <span className="found-meta">#{chosen.id}</span>
          </span>
          <button type="button" className="quiet" onClick={() => pick("")}>
            Change
          </button>
        </p>
      </div>
    );

  const words = text.toLowerCase().split(/\s+/).filter(Boolean);
  const matches = candidates.filter((c) => {
    const hay = `${c.title} #${c.id} ${c.project ?? ""} ${c.cellName ?? ""} ${c.typeName}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
  return (
    <div className="finder">
      <label htmlFor="find-item">What is the new priority?</label>
      <input
        id="find-item"
        type="search"
        autoFocus
        placeholder="Search by title, number or project"
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
      <ul className="found">
        {matches.slice(0, SHOWN).map((c) => (
          <li key={c.id}>
            <button type="button" onClick={() => pick(String(c.id))}>
              <TypeIcon icon={c.icon} name={c.typeName} />
              <span className="found-title">{c.title}</span>
              <span className="found-meta">
                #{c.id} · {c.stage === "intake" ? "not yet queued" : `queued in ${c.cellName ?? "a cell"}`}
                {c.project && ` · ${c.project}`}
              </span>
            </button>
          </li>
        ))}
        {matches.length > SHOWN && <li className="found-more">{matches.length - SHOWN} more. Keep typing to narrow it down.</li>}
        {matches.length === 0 && (
          <li className="found-more">Nothing matches. If it has only just been created, use Check now, then search again.</li>
        )}
      </ul>
    </div>
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
  const free = p.lane.freeHands;
  const [pausing, setPausing] = useState(free === 0);
  const cellName = p.lane.cell.name;
  const weeks = ws.horizonWeeks;

  let cost: string;
  if (p.alreadyAbove) cost = `It is already within the ${weeks}-week line, so nothing falls past it.`;
  else if (p.falls) cost = `It pushes one item past the ${weeks}-week line:`;
  else if (!p.lane.measured) cost = `${cellName} has no finished work to measure pace from yet, so there is no line to cross.`;
  else if (p.lane.line === 0) cost = `Work already in progress fills ${cellName}'s next ${weeks} weeks, so even at the front it waits.`;
  else cost = `${cellName} has room within the ${weeks}-week line. Nothing falls past it.`;

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
        <h2>What it costs to put it at the front of the {cellName} queue</h2>
        <ul className="cost">
          <li>
            {cost}
            {p.falls && <CardView plain card={p.falls} owner={false} />}
          </li>
          {p.behind > 0 && (
            <li>
              {p.behind === 1 ? "One queued item moves" : `${p.behind} queued items move`} back a place
              {p.delay && `, each starting about ${p.delay} later`}.
            </li>
          )}
          {p.atRisk.map((c) => (
            <li key={c.id} className="stop">
              <b className="stop">{c.risk}</b>
              <CardView plain card={c} owner={false} />
            </li>
          ))}
          {p.behind > 0 && p.atRisk.length === 0 && <li>No date is put at risk.</li>}
        </ul>
      </section>

      <section>
        <h2>When it can start</h2>
        {free > 0 && (
          <p>
            {cellName} has {free === 1 ? "a free hand" : `${free} free hands`}, so it can start now. Nothing needs to stop.{" "}
            {!pausing && p.pause.length > 0 && (
              <button type="button" className="quiet" onClick={() => setPausing(true)}>
                Stop something anyway
              </button>
            )}
          </p>
        )}
        {free === 0 && p.pause.length === 0 && <p>Nothing is in progress in {cellName}.</p>}
        {free === 0 && p.pause.length > 0 && (
          <p>Every hand in {cellName} is full. It waits for the next one to come free, unless something stops to make room. Cheapest to stop first:</p>
        )}
        {pausing && p.pause.length > 0 && (
          <div className="choices">
            <label className="choice">
              <input type="radio" name="pause" checked={pause === null} onChange={() => setPause(null)} />
              <span>Stop nothing. It takes the next free hand.</span>
            </label>
            {p.pause.map(({ card, note }) => (
              <label key={card.id} className="choice">
                <input type="radio" name="pause" checked={pause === card.id} onChange={() => setPause(card.id)} />
                <span>
                  <CardView plain card={card} />
                  {note && <span className="pause-note">{note}</span>}
                </span>
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
            {pause === null ? "Record decision" : "Record decision and stop that work"}
          </button>
          {problem && <p className="notice stop">{problem}</p>}
        </section>
      ) : (
        <p className="empty">You can explore the cost here. Recording a decision is for managers.</p>
      )}
    </form>
  );
}
