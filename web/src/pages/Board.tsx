import { type Floor, type Lane, type Workspace, useData } from "../api";
import { type Go, Link } from "../App";
import { CardView } from "../CardView";

const SHOWN = 6; // cards per column before the rest are summarised

export function Board({ ws, go }: { ws: Workspace; go: Go }) {
  const { data, error } = useData<Floor>(`/api/w/${ws.slug}/floor`, 30_000);
  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;
  const s = data.signals;
  return (
    <>
      <section className="signals" aria-label="What needs attention">
        <Link to={`/w/${ws.slug}/triage`} go={go} className={`signal${s.triage ? " hot" : ""}`}>
          <b>{s.triage}</b> to triage
        </Link>
        <span className={`signal${s.blocked ? " hot" : ""}`}>
          <b>{s.blocked}</b> blocked
        </span>
        <span className={`signal${s.aging ? " warm" : ""}`}>
          <b>{s.aging}</b> aging
        </span>
        <span className={`signal${s.over ? " hot" : ""}`}>
          <b>{s.over}</b> over their limit
        </span>
      </section>
      {data.lanes.map((lane) => (
        <LaneView key={lane.cell.id} lane={lane} ws={ws} go={go} />
      ))}
    </>
  );
}

function LaneView({ lane, ws, go }: { lane: Lane; ws: Workspace; go: Go }) {
  const above = lane.queue.slice(0, lane.line);
  const next = above.slice(0, SHOWN);
  return (
    <section className="lane">
      <header className="lane-head">
        <h2>{lane.cell.name}</h2>
        <Pace lane={lane} />
        <ul className="people">
          {lane.people.map((p) => (
            <li key={p.name} className={p.count > p.limit ? "over" : undefined}>
              {p.name}
              <span className="dots" role="img" aria-label={`${p.count} in progress, limit ${p.limit}`}>
                {Array.from({ length: Math.max(p.count, p.limit) }, (_, k) => (
                  <i key={k} className={k >= p.limit ? "over" : k < p.count ? "on" : undefined} />
                ))}
              </span>
            </li>
          ))}
        </ul>
      </header>
      <div className="columns">
        <div className="column">
          <h3>
            Next up
            <span className="count">
              {above.length} of {lane.queue.length} queued
            </span>
          </h3>
          {next.map((c) => (
            <CardView key={c.id} card={c} owner={false} />
          ))}
          {above.length === 0 && (
            <p className="empty">
              {lane.queue.length === 0
                ? "The queue is empty."
                : lane.measured
                  ? `Work in progress already fills the next ${ws.horizonWeeks} weeks.`
                  : "No finished work yet to measure pace from."}
            </p>
          )}
          {lane.queue.length > next.length && (
            <Link to={`/w/${ws.slug}/queues`} go={go} className="more">
              Full queue
            </Link>
          )}
        </div>
        <div className="column">
          <h3>
            In progress
            {/* A cell with nobody configured has no limit to compare against. */}
            <span className={`count${lane.wipLimit > 0 && lane.doing.length > lane.wipLimit ? " stop" : ""}`}>
              {lane.doing.length}
              {lane.wipLimit > 0 && ` / ${lane.wipLimit}`}
            </span>
          </h3>
          {lane.doing.map((c) => (
            <CardView key={c.id} card={c} />
          ))}
          {lane.doing.length === 0 && <p className="empty">Nothing in progress.</p>}
        </div>
        <div className="column">
          <h3>
            Done <span className="count">since last standup</span>
          </h3>
          {lane.done.slice(0, SHOWN).map((c) => (
            <CardView key={c.id} card={c} />
          ))}
          {lane.done.length > SHOWN && <p className="more">+ {lane.done.length - SHOWN} more</p>}
          {lane.quickDone > 0 && (
            <p className="more">
              + {lane.quickDone} quick ticket{lane.quickDone === 1 ? "" : "s"}
            </p>
          )}
          {lane.done.length === 0 && lane.quickDone === 0 && <p className="empty">Nothing finished yet.</p>}
        </div>
      </div>
    </section>
  );
}

export function Pace({ lane }: { lane: Lane }) {
  return (
    <p className="pace">
      <span>
        <b>{lane.measured ? lane.throughput.toFixed(1) : "–"}</b> finished / week
      </span>
      <span>
        <b>{Math.round(lane.quickRate)}</b> quick ticket{Math.round(lane.quickRate) === 1 ? "" : "s"} / week
      </span>
    </p>
  );
}
