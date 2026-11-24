import { Fragment, useEffect, useState } from "react";
import { type Card, type Floor, type Hand, type Lane, type Workspace, ApiError, post, useData } from "../api";
import { type Go, Link } from "../App";
import { TimeBar, TypeMark, useDetail } from "../Detail";
import { TypeIcon } from "../TypeIcon";

const ON_THE_LINE = 12; // queued items drawn on the line before the rest are summarised

/** The team's floor: who holds what, who has a free hand, and what is coming down the line. */
export function FloorPage({ ws, go }: { ws: Workspace; go: Go }) {
  const { data, error, reload } = useData<Floor>(`/api/w/${ws.slug}/floor`, 30_000);
  const [focus, setFocus] = useState<string | null>(null);
  const [pulse, setPulse] = useState<number | null>(null);
  const [allAlerts, setAllAlerts] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    if (pulse === null) return;
    document.querySelector(`.room [data-item="${pulse}"]`)?.scrollIntoView({ block: "center", behavior: "smooth" });
    const t = setTimeout(() => setPulse(null), 2600);
    return () => clearTimeout(t);
  }, [pulse]);

  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;

  const move = async (cell: string, id: number, before: number | null) => {
    try {
      await post(`/api/w/${ws.slug}/queues/${cell}/move`, { id, before });
      setNotice(null);
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Could not save the new order.");
    }
    reload();
  };

  const alerts = allAlerts ? data.attention : data.attention.slice(0, 6);
  const types = data.lanes[0]?.mix ?? [];
  const look = { focus, pulse };

  return (
    <>
      <section className="attention" aria-label="Needs attention">
        <h1>{data.attention.length === 0 ? "Nothing needs attention" : "Needs attention"}</h1>
        {data.attention.length === 0 && <p className="lede">Everything in progress is moving, and no date is at risk.</p>}
        <ol>
          {alerts.map((a, n) => (
            <li key={n} className={a.level}>
              <button type="button" className="alert" onClick={() => a.id && setPulse(a.id)} disabled={!a.id}>
                <span className="alert-dot" aria-hidden="true" />
                <span>{a.text}</span>
                {a.cell && <span className="alert-cell">{a.cell}</span>}
              </button>
            </li>
          ))}
        </ol>
        <p className="attention-foot">
          {data.attention.length > 6 && (
            <button type="button" className="quiet" onClick={() => setAllAlerts(!allAlerts)}>
              {allAlerts ? "Show fewer" : `Show all ${data.attention.length}`}
            </button>
          )}
          {data.signals.triage > 0 && (
            <Link to={`/w/${ws.slug}/triage`} go={go} className="more">
              {data.signals.triage} waiting for triage
            </Link>
          )}
          {data.signals.lead && <span className="lede">Triage lead this week: {data.signals.lead}</span>}
        </p>
      </section>

      <div className="chips" role="group" aria-label="Highlight a type of work">
        <button type="button" className={focus === null ? "chip on" : "chip"} onClick={() => setFocus(null)}>
          All work
        </button>
        {types.map((t) => (
          <button key={t.id} type="button" className={focus === t.id ? "chip on" : "chip"} onClick={() => setFocus(focus === t.id ? null : t.id)}>
            <TypeIcon icon={t.icon} />
            {t.name}
          </button>
        ))}
      </div>
      {notice && <p className="notice stop">{notice}</p>}

      {data.lanes.map((lane) => (
        <Room key={lane.cell.id} lane={lane} ws={ws} go={go} look={look} move={move} />
      ))}
    </>
  );
}

type Look = { focus: string | null; pulse: number | null };

const state = (c: Card, look: Look) =>
  [look.focus && c.type !== look.focus && "dim", look.pulse === c.id && "pulse"].filter(Boolean).join(" ");

function Room(props: {
  lane: Lane;
  ws: Workspace;
  go: Go;
  look: Look;
  move: (cell: string, id: number, before: number | null) => void;
}) {
  const { lane, look } = props;
  const held = new Set(lane.hands.flatMap((h) => h.items.map((c) => c.id)));
  const loose = lane.doing.filter((c) => !held.has(c.id));
  const busy = lane.doing.length;

  return (
    <section className="room">
      <header className="room-head">
        <h2>{lane.cell.name}</h2>
        <dl className="gauges">
          {lane.wipLimit > 0 && (
            <div>
              <dt>Hands</dt>
              <dd className={busy > lane.wipLimit ? "stop" : undefined}>
                <b>{busy}</b> of {lane.wipLimit} busy{lane.freeHands > 0 && `, ${lane.freeHands} free`}
              </dd>
            </div>
          )}
          <div>
            <dt>Pace</dt>
            <dd>
              <b>{lane.measured ? lane.throughput.toFixed(1) : "–"}</b> finished a week
            </dd>
          </div>
          <div>
            <dt>Unplanned this week</dt>
            <dd className={lane.arrivals >= 3 && lane.arrivals > lane.arrivalsUsual * 1.5 ? "warn" : undefined}>
              <b>{lane.arrivals}</b> arrived, usually {Math.round(lane.arrivalsUsual)}
            </dd>
          </div>
        </dl>
      </header>

      <div className="mats">
        {lane.hands.map((h) => (
          <Mat key={h.login} hand={h} look={look} />
        ))}
        {loose.length > 0 && (
          <Mat hand={{ login: "", name: "Not assigned", limit: loose.length, items: loose, waiting: [], pull: null }} look={look} />
        )}
      </div>

      {lane.quick.length > 0 && <QuickList lane={lane} look={look} />}
      <TheLine {...props} />
      <Mix lane={lane} look={look} />
    </section>
  );
}

/** Small jobs kept off the line. Anyone with a spare half hour takes the oldest. */
function QuickList({ lane, look }: { lane: Lane; look: Look }) {
  const open = useDetail();
  return (
    <div className="quick">
      <p className="line-head">
        <b>Quick ones</b>
        <span>{lane.quick.length} waiting. For a spare half hour, oldest first.</span>
      </p>
      <ul>
        {lane.quick.map((c) => (
          <li key={c.id}>
            <button type="button" data-item={c.id} className={`quick-item ${c.risk ? "late" : ""} ${state(c, look)}`} onClick={() => open(c)}>
              <TypeMark card={c} />
              {c.title}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** One person's mat. Each hand is a slot: holding something, or free with a suggestion from the line. */
function Mat({ hand, look }: { hand: Hand; look: Look }) {
  const open = useDetail();
  const free = Math.max(0, hand.limit - hand.items.length);
  return (
    <div className="mat">
      <p className="nafuda">{hand.name}</p>
      <div className="slots">
        {hand.items.map((c, n) => (
          <button
            key={c.id}
            type="button"
            data-item={c.id}
            className={`slot held ${c.blocked ? "is-blocked" : ""} ${n >= hand.limit ? "extra" : ""} ${state(c, look)}`}
            onClick={() => open(c)}
          >
            <span className="slot-title">
              <TypeMark card={c} />
              {c.title}
            </span>
            <span className={`slot-note${c.blocked ? " stop" : c.agingFor || c.quietFor ? " warn" : ""}`}>
              {c.blocked
                ? `Blocked ${c.blockedFor ?? ""}`
                : c.quietFor
                  ? `Quiet ${c.quietFor}`
                  : n >= hand.limit
                    ? "Beyond the limit"
                    : c.day
                      ? `Day ${c.day}`
                      : ""}
            </span>
            <TimeBar card={c} />
          </button>
        ))}
        {Array.from({ length: free }, (_, n) =>
          hand.pull && n === 0 ? (
            <button key={`f${n}`} type="button" className={`slot free ${state(hand.pull, look)}`} onClick={() => open(hand.pull!)}>
              <span className="slot-note">Free hand. Next on the line:</span>
              <span className="slot-title">
                <TypeMark card={hand.pull} />
                {hand.pull.title}
              </span>
            </button>
          ) : (
            <span key={`f${n}`} className="slot free empty">
              <span className="slot-note">Free hand</span>
            </span>
          ),
        )}
      </div>
      {hand.waiting.length > 0 && (
        <ul className="shelf" aria-label="Waiting on others">
          {hand.waiting.map((c) => (
            <li key={c.id}>
              <button type="button" data-item={c.id} className={`shelved ${c.chase ? "chase" : ""} ${state(c, look)}`} onClick={() => open(c)}>
                <span className="shelved-title">{c.title}</span>
                <span className="shelved-note">
                  Waiting {c.waitingFor} on {c.waitingOn}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** A cell's queue drawn as a line through time. Drag an item to change what comes first. */
function TheLine(props: {
  lane: Lane;
  ws: Workspace;
  go: Go;
  look: Look;
  move: (cell: string, id: number, before: number | null) => void;
}) {
  const { lane, ws, go, look, move } = props;
  const open = useDetail();
  const q = lane.queue.slice(0, ON_THE_LINE);
  const [dragging, setDragging] = useState<number | null>(null);
  const [target, setTarget] = useState<number | null | undefined>(undefined);

  const when = (c: Card) => {
    if (c.waitDays === undefined) return "In order";
    if (c.waitDays === 0) return "Now";
    if (c.waitDays <= 7) return "This week";
    if (c.waitDays <= ws.horizonWeeks * 7) return `Within ${ws.horizonWeeks} weeks`;
    return "Later";
  };
  const end = () => {
    setDragging(null);
    setTarget(undefined);
  };

  return (
    <div className="line">
      <p className="line-head">
        <b>The line</b>
        <span>
          {lane.queue.length} queued{ws.canWrite && lane.queue.length > 1 && ". Drag to change what comes first."}
        </span>
      </p>
      {lane.queue.length === 0 ? (
        <p className="empty">Nothing is queued.</p>
      ) : (
        <ol
          className="track"
          onDragOver={(e) => dragging !== null && e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault();
            if (dragging !== null && target !== undefined && target !== dragging) move(lane.cell.id, dragging, target);
            end();
          }}
        >
          {q.map((c, n) => (
            <Fragment key={c.id}>
              {(n === 0 || when(q[n - 1]) !== when(c)) && <li className="marker">{when(c)}</li>}
              <li
                data-item={c.id}
                className={`tile ${c.risk ? "at-risk" : ""} ${c.cos === "expedite" ? "is-expedite" : ""} ${dragging === c.id ? "dragging" : ""} ${target === c.id && dragging !== c.id ? "drop-before" : ""} ${state(c, look)}`}
                draggable={ws.canWrite}
                onDragStart={(e) => {
                  e.dataTransfer.effectAllowed = "move";
                  setDragging(c.id);
                }}
                onDragEnd={end}
                onDragOver={(e) => {
                  if (dragging === null) return;
                  const box = e.currentTarget.getBoundingClientRect();
                  setTarget(e.clientX < box.left + box.width / 2 ? c.id : (lane.queue[n + 1]?.id ?? null));
                }}
                onClick={() => open(c)}
              >
                <span className="tile-top">
                  <span className="num">{n + 1}</span>
                  <TypeMark card={c} />
                </span>
                <span className="tile-title">{c.title}</span>
                {c.risk ? (
                  <span className="tile-note stop">At risk: due {c.due}</span>
                ) : c.cos === "expedite" ? (
                  <span className="tile-note stop">Expedite</span>
                ) : c.due ? (
                  <span className="tile-note">Due {c.due}</span>
                ) : null}
              </li>
            </Fragment>
          ))}
          {lane.queue.length > q.length && (
            <li className="tile more-tile">
              <Link to={`/w/${ws.slug}/queues`} go={go}>
                + {lane.queue.length - q.length} more
              </Link>
            </li>
          )}
        </ol>
      )}
    </div>
  );
}

/** Where the cell's finished work went, by type. One series, so one colour. */
function Mix({ lane, look }: { lane: Lane; look: Look }) {
  const total = lane.mix.reduce((n, s) => n + s.count, 0);
  if (total === 0) return null;
  return (
    <div className="mix" role="img" aria-label="Share of finished work by type over recent weeks">
      <p className="mix-head">Where finished work went</p>
      {lane.mix.map((s) => (
        <div key={s.id} className={`mix-row ${look.focus && look.focus !== s.id ? "dim" : ""}`} title={`${s.count} of ${total} items`}>
          <span className="mix-name">
            <TypeIcon icon={s.icon} />
            {s.name}
          </span>
          <span className="mix-bar">
            <i className={s.id === "unplanned" ? "unplanned" : undefined} style={{ width: `${s.pct}%` }} />
          </span>
          <span className="mix-pct">{Math.round(s.pct)}%</span>
        </div>
      ))}
    </div>
  );
}
