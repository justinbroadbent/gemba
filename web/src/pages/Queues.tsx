import { Fragment, useState } from "react";
import { type Floor, type Lane, type Workspace, ApiError, post, useData } from "../api";
import { CardView } from "../CardView";

export function Queues({ ws }: { ws: Workspace }) {
  const { data, error, reload } = useData<Floor>(`/api/w/${ws.slug}/floor`, 30_000);
  const [notice, setNotice] = useState<string | null>(null);

  const move = async (cell: string, id: number, before: number | null) => {
    try {
      await post(`/api/w/${ws.slug}/queues/${cell}/move`, { id, before });
      setNotice(null);
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Could not save the new order.");
    }
    reload();
  };

  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;
  return (
    <>
      <header className="page-head">
        <h1>Queues</h1>
        <p>
          The top of each queue is pulled next. The line marks what fits in the next {ws.horizonWeeks} weeks at the
          measured pace.{ws.canWrite && " Drag to reorder."}
        </p>
      </header>
      {notice && <p className="notice stop">{notice}</p>}
      <div className="queues">
        {data.lanes.map((lane) => (
          <QueueColumn key={lane.cell.id} lane={lane} ws={ws} move={move} />
        ))}
      </div>
    </>
  );
}

function QueueColumn(props: {
  lane: Lane;
  ws: Workspace;
  move: (cell: string, id: number, before: number | null) => void;
}) {
  const { lane, ws, move } = props;
  const q = lane.queue;
  const [dragging, setDragging] = useState<number | null>(null);
  const [target, setTarget] = useState<number | null | undefined>(undefined); // id to drop before; null = end

  const drop = () => {
    if (dragging !== null && target !== undefined && target !== dragging) move(lane.cell.id, dragging, target);
    setDragging(null);
    setTarget(undefined);
  };

  const capacity = (
    <li className="capacity" key="capacity">
      {lane.measured ? `${ws.horizonWeeks}-week capacity` : "No pace measured yet"}
      {lane.line > q.length && ` · room for ${lane.line - q.length} more`}
    </li>
  );

  return (
    <section className="queue-col">
      <h2>{lane.cell.name}</h2>
      <p className="pace">
        <span>
          <b>{lane.measured ? lane.throughput.toFixed(1) : "–"}</b> / week
        </span>
        <span>
          <b>{lane.doing.length}</b> in progress
        </span>
        <span>
          <b>{q.length}</b> queued
        </span>
      </p>
      <ol
        className="queue"
        onDragOver={(e) => dragging !== null && e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault();
          drop();
        }}
      >
        {q.map((card, n) => (
          <Fragment key={card.id}>
            {n === lane.line && capacity}
            <li
              className={[dragging === card.id && "dragging", target === card.id && dragging !== card.id && "drop-before"]
                .filter(Boolean)
                .join(" ")}
              draggable={ws.canWrite}
              onDragStart={(e) => {
                e.dataTransfer.effectAllowed = "move";
                setDragging(card.id);
              }}
              onDragEnd={() => {
                setDragging(null);
                setTarget(undefined);
              }}
              onDragOver={(e) => {
                if (dragging === null) return;
                const box = e.currentTarget.getBoundingClientRect();
                const upper = e.clientY < box.top + box.height / 2;
                setTarget(upper ? card.id : (q[n + 1]?.id ?? null));
              }}
            >
              <CardView card={card} owner={false} dim={n >= lane.line} />
              {ws.canWrite && (
                <span className="nudge">
                  <button
                    type="button"
                    aria-label={`Move #${card.id} up`}
                    disabled={n === 0}
                    onClick={() => move(lane.cell.id, card.id, q[n - 1].id)}
                  >
                    ↑
                  </button>
                  <button
                    type="button"
                    aria-label={`Move #${card.id} down`}
                    disabled={n === q.length - 1}
                    onClick={() => move(lane.cell.id, card.id, q[n + 2]?.id ?? null)}
                  >
                    ↓
                  </button>
                </span>
              )}
            </li>
          </Fragment>
        ))}
        {target === null && dragging !== null && <li className="drop-end" aria-hidden="true" />}
        {lane.line >= q.length && capacity}
        {q.length === 0 && <li className="empty">The queue is empty.</li>}
      </ol>
    </section>
  );
}
