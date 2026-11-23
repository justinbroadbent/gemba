import { useState } from "react";
import { type Inbox, type Session, type Workspace, ApiError, label, post, useData } from "../api";

type Row = Inbox["rows"][number];

export function Triage({ ws, session }: { ws: Workspace; session: Session }) {
  const { data, error, reload } = useData<Inbox>(`/api/w/${ws.slug}/inbox`, 30_000);
  const [notice, setNotice] = useState<string | null>(null);
  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;

  const queue = async (id: number, cell: string, cos: string, top: boolean) => {
    try {
      await post(`/api/w/${ws.slug}/inbox/${id}`, { cell, cos, top });
      setNotice(null);
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Could not queue the item.");
    }
    reload();
  };

  return (
    <>
      <header className="page-head">
        <h1>Triage</h1>
        <p>
          {data.rows.length} need{data.rows.length === 1 ? "s" : ""} a decision.{" "}
          {data.fastLane > 0 &&
            `${data.fastLane} fresh ticket${data.fastLane === 1 ? " is" : "s are"} still in the fast lane and may resolve without one.`}
        </p>
      </header>
      {notice && <p className="notice stop">{notice}</p>}
      {data.rows.length === 0 && <p className="empty big">Nothing to triage.</p>}
      {data.rows.map((row) => (
        <TriageRow key={row.card.id} row={row} ws={ws} cos={session.cos} queue={queue} />
      ))}
    </>
  );
}

function TriageRow(props: {
  row: Row;
  ws: Workspace;
  cos: string[];
  queue: (id: number, cell: string, cos: string, top: boolean) => Promise<void>;
}) {
  const { row, ws } = props;
  const [cell, setCell] = useState(row.card.cell ?? "");
  const [cos, setCos] = useState(row.urgent ? "expedite" : "standard");
  const [top, setTop] = useState(row.urgent);
  const [busy, setBusy] = useState(false);
  return (
    <form
      className={`triage${row.urgent ? " is-expedite" : ""}`}
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        await props.queue(row.card.id, cell, cos, top);
        setBusy(false);
      }}
    >
      <div className="triage-what">
        <p className="card-title">
          {row.card.url ? (
            <a href={row.card.url} target="_blank" rel="noreferrer">
              {row.card.title}
            </a>
          ) : (
            row.card.title
          )}
        </p>
        <p className="card-meta">
          <span className="num">#{row.card.id}</span>
          <span className={row.urgent ? "flag stop" : undefined}>{row.why}</span>
          {row.priority && !row.urgent && <span>Source priority {row.priority}</span>}
        </p>
      </div>
      {ws.canWrite && (
        <div className="triage-how">
          <select aria-label="Cell" required value={cell} onChange={(e) => setCell(e.target.value)}>
            <option value="" disabled>
              Cell
            </option>
            {ws.cells.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
          <select aria-label="Class of service" value={cos} onChange={(e) => setCos(e.target.value)}>
            {props.cos.map((c) => (
              <option key={c} value={c}>
                {label(c)}
              </option>
            ))}
          </select>
          <select aria-label="Position in queue" value={top ? "top" : "bottom"} onChange={(e) => setTop(e.target.value === "top")}>
            <option value="bottom">Back of queue</option>
            <option value="top">Front of queue</option>
          </select>
          <button type="submit" disabled={busy}>
            Queue
          </button>
        </div>
      )}
    </form>
  );
}
