import { Fragment, useState } from "react";
import { type Floor, type Lane, type Workspace, ApiError, post, useData } from "../api";
import { CardView } from "../CardView";

/** Every queued item, for when the line on the floor is not enough. Built to hold hundreds. */
export function Queues({ ws }: { ws: Workspace }) {
  const { data, error, reload } = useData<Floor>(`/api/w/${ws.slug}/floor`, 30_000);
  const [notice, setNotice] = useState<string | null>(null);
  const [find, setFind] = useState("");

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
      <header className="page-head with-tools">
        <div>
          <h1>Queues</h1>
          <p>
            The top of each queue is pulled next. The line marks what fits in the next {ws.horizonWeeks} weeks at the
            measured pace.{ws.canWrite && " Drag to reorder."}
          </p>
        </div>
        <input
          type="search"
          aria-label="Find in the queues"
          placeholder="Find by title, project or type"
          value={find}
          onChange={(e) => setFind(e.target.value)}
        />
      </header>
      {notice && <p className="notice stop">{notice}</p>}
      <div className="queues">
        {data.lanes.map((lane) => (
          <QueueColumn key={lane.cell.id} lane={lane} ws={ws} move={move} find={find.trim().toLowerCase()} />
        ))}
      </div>
    </>
  );
}

function QueueColumn(props: {
  lane: Lane;
  ws: Workspace;
  find: string;
  move: (cell: string, id: number, before: number | null) => void;
}) {
  const { lane, ws, move, find } = props;
  const q = lane.queue;
  const [dragging, setDragging] = useState<number | null>(null);
  const [target, setTarget] = useState<number | null | undefined>(undefined); // id to drop before; null = end
  const [showAll, setShowAll] = useState(false);

  // Past the line is folded away by default: with hundreds queued, it is the long tail.
  const past = Math.max(0, q.length - lane.line);
  const foldAt = find || showAll ? q.length : Math.max(lane.line, 3);
  const matches = (n: number) =>
    !find || `${q[n].title} ${q[n].project ?? ""} ${q[n].typeName} #${q[n].id}`.toLowerCase().includes(find);
  const canDrag = ws.canWrite && !find;

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
          <b>{lane.measured ? lane.throughput.toFixed(1) : "–"}</b> a week
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
        {q.map((card, n) =>
          n >= foldAt || !matches(n) ? null : (
            <Fragment key={card.id}>
              {n === lane.line && !find && capacity}
              <li
                className={[dragging === card.id && "dragging", target === card.id && dragging !== card.id && "drop-before"]
                  .filter(Boolean)
                  .join(" ")}
                draggable={canDrag}
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
                  setTarget(e.clientY < box.top + box.height / 2 ? card.id : (q[n + 1]?.id ?? null));
                }}
              >
                <CardView card={card} owner={false} dim={n >= lane.line} />
                {canDrag && (
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
          ),
        )}
        {target === null && dragging !== null && <li className="drop-end" aria-hidden="true" />}
        {/* The line is also drawn when it falls exactly where the list folds. */}
        {(lane.line >= q.length || lane.line >= foldAt) && !find && capacity}
        {q.length === 0 && <li className="empty">The queue is empty.</li>}
      </ol>
      {!find && q.length > Math.max(lane.line, 3) && (
        <button type="button" className="quiet" onClick={() => setShowAll(!showAll)}>
          {showAll ? "Fold away what is past the line" : `Show all ${past} past the line`}
        </button>
      )}
    </section>
  );
}
