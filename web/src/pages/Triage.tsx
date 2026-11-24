import { useState } from "react";
import { TypeIcon } from "../TypeIcon";
import { type CellOutlook, type Inbox, type Session, type Workspace, ApiError, label, post, useData } from "../api";

type Row = Inbox["rows"][number];
type Choice = { cell: string; cos: string; kind: string; top: boolean; due: string };
type Queue = (id: number, c: Choice) => Promise<boolean>;

/**
 * Two jobs share this page. Tickets arrive unasked and need a quick decision.
 * Planned work arrives in batches when someone breaks a project down, and is
 * stacked into a queue a batch at a time.
 */
export function Triage({ ws, session }: { ws: Workspace; session: Session }) {
  const { data, error, reload } = useData<Inbox>(`/api/w/${ws.slug}/inbox`, 30_000);
  const [notice, setNotice] = useState<string | null>(null);
  const [type, setType] = useState<string | null>(null);
  const [picked, setPicked] = useState<number | null>(null);
  const [early, setEarly] = useState(false);
  const [rotaOpen, setRotaOpen] = useState(false);
  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;

  const queue: Queue = async (id, c) => {
    try {
      await post(`/api/w/${ws.slug}/inbox/${id}`, c);
      setNotice(null);
      return true;
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Could not queue the item.");
      return false;
    }
  };
  const saveRota = async (change: { rotation?: string[]; weeks?: Record<string, string> }) => {
    try {
      await post(`/api/w/${ws.slug}/triage-rota`, change);
      setNotice(null);
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Could not change the rota.");
    }
    reload();
  };
  const handOver = async (login: string) => {
    try {
      await post(`/api/w/${ws.slug}/triage-lead`, { login });
      setNotice(null);
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Could not change the triage lead.");
    }
    reload();
  };

  // The types present, in a steady order, for the filter.
  const types = new Map<string, { name: string; icon: string; count: number }>();
  for (const r of data.rows) {
    const t = types.get(r.card.type) ?? { name: r.card.typeName, icon: r.card.icon, count: 0 };
    t.count++;
    types.set(r.card.type, t);
  }
  const rows = data.rows.filter((r) => !type || r.card.type === type);
  const tickets = rows.filter((r) => r.card.mirrored);
  const planned = rows.filter((r) => !r.card.mirrored);
  const batches = new Map<string, Row[]>();
  for (const r of planned) {
    const name = r.card.project ?? "";
    batches.set(name, [...(batches.get(name) ?? []), r]);
  }
  const fresh = early ? data.fresh.filter((r) => !type || r.card.type === type) : [];
  const open = picked !== null && [...rows, ...fresh].some((r) => r.card.id === picked) ? picked : rows[0]?.card.id;
  const shared = { ws, data, cos: session.cos, queue, reload, open, setOpen: setPicked };

  return (
    <>
      <header className="page-head with-tools">
        <div>
          <h1>Triage</h1>
          <p>
            {data.rows.length === 0 ? "Nothing needs a decision." : `${data.rows.length} need${data.rows.length === 1 ? "s" : ""} a decision.`}
            {data.late > 0 && (
              <b className="stop">
                {" "}
                {data.late} {data.late === 1 ? "has" : "have"} waited longer than the {data.targetHours} h target.
              </b>
            )}
            {data.fastLane > 0 &&
              ` ${data.fastLane} fresh ticket${data.fastLane === 1 ? " is" : "s are"} still in the fast lane and may be resolved without one.`}{" "}
            {data.fresh.length > 0 && (
              <button type="button" className="quiet" onClick={() => setEarly(!early)}>
                {early ? "Hide them" : "Triage one now"}
              </button>
            )}
          </p>
        </div>
        <LeadPlate data={data} handOver={handOver} showRota={() => setRotaOpen(!rotaOpen)} />
      </header>
      {notice && <p className="notice stop">{notice}</p>}
      {rotaOpen && data.rota && <RotaPanel data={data} save={saveRota} />}

      <section className="landing" aria-label="Where work would land">
        {data.cells.map((c) => (
          <div key={c.id} className="landing-cell">
            <b>{c.name}</b>
            <span>
              {c.queued} queued, {c.quick} quick, {c.free === 0 ? "no free hands" : `${c.free} free hand${c.free === 1 ? "" : "s"}`}
            </span>
            <span className="landing-when">{c.back ? `Back of the queue: ${lower(c.back)}` : "No pace measured yet"}</span>
          </div>
        ))}
      </section>

      {types.size > 1 && (
        <div className="chips" role="group" aria-label="Show one type of work">
          <button type="button" className={type === null ? "chip on" : "chip"} onClick={() => setType(null)}>
            All {data.rows.length}
          </button>
          {[...types].map(([id, t]) => (
            <button key={id} type="button" className={type === id ? "chip on" : "chip"} onClick={() => setType(type === id ? null : id)}>
              <TypeIcon icon={t.icon} />
              {t.name} {t.count}
            </button>
          ))}
        </div>
      )}

      {rows.length === 0 && fresh.length === 0 && <p className="empty big">Nothing to triage.</p>}

      {fresh.length > 0 && (
        <section className="pile">
          <h2 className="eyebrow">
            Fresh tickets <span className="count">In the fast lane. Leave them unless you already know where one belongs</span>
          </h2>
          {fresh.map((r) => (
            <TriageRow key={r.card.id} row={r} {...shared} />
          ))}
        </section>
      )}

      {tickets.length > 0 && (
        <section className="pile">
          <h2 className="eyebrow">
            Tickets <span className="count">Unplanned work that has not been resolved on its own</span>
          </h2>
          {tickets.map((r) => (
            <TriageRow key={r.card.id} row={r} {...shared} />
          ))}
        </section>
      )}

      {[...batches].map(([project, batch]) => (
        <section key={project} className="pile">
          <h2 className="eyebrow">
            {project || "Planned work"}
            <span className="count">{project ? `${batch.length} planned, not yet queued` : "Not part of a project, not yet queued"}</span>
          </h2>
          {batch.length > 1 && data.canTriage && <Batch key={batch.map((r) => r.card.id).join()} batch={batch} {...shared} />}
          {batch.map((r) => (
            <TriageRow key={r.card.id} row={r} {...shared} />
          ))}
        </section>
      ))}
    </>
  );
}

const lower = (s: string) => s.charAt(0).toLowerCase() + s.slice(1);

/** Who answers for triage this week. Everyone sees it; a manager hands it over. */
function LeadPlate({ data, handOver, showRota }: { data: Inbox; handOver: (login: string) => void; showRota: () => void }) {
  const [changing, setChanging] = useState(false);
  const lead = data.lead;
  return (
    <div className="lead">
      <p className="lead-label">Triage lead this week</p>
      {changing ? (
        <select
          aria-label="Triage lead"
          autoFocus
          defaultValue=""
          onBlur={() => setChanging(false)}
          onChange={(e) => {
            setChanging(false);
            handOver(e.target.value);
          }}
        >
          <option value="" disabled>
            Hand over to
          </option>
          {data.people.map((p) => (
            <option key={p.login} value={p.login}>
              {p.name}
            </option>
          ))}
        </select>
      ) : (
        <p className={lead ? "lead-name" : "lead-name nobody"}>{lead ? lead.name : "Nobody yet"}</p>
      )}
      <p className="lead-note">
        {lead && (
          <span className={lead.source === "assigned" && lead.days >= 7 ? "warn" : undefined}>
            {lead.source === "stand-in"
              ? `Standing in since ${lead.since}`
              : lead.source === "assigned" && lead.days >= 7
                ? `Held for ${lead.days} days`
                : lead.source === "assigned"
                  ? `Since ${lead.since}`
                  : "By the rota"}
          </span>
        )}
        {data.canAssign && !changing && (
          <button type="button" className="quiet" onClick={() => setChanging(true)}>
            {lead ? "Stand-in" : "Name someone"}
          </button>
        )}
        {data.rota && (
          <button type="button" className="quiet" onClick={showRota}>
            Rota
          </button>
        )}
      </p>
    </div>
  );
}

/**
 * Who leads triage in the coming weeks. Everyone can see it. A manager sets the
 * rotation, and can give any week to a particular person.
 */
function RotaPanel({ data, save }: { data: Inbox; save: (change: { rotation?: string[]; weeks?: Record<string, string> }) => void }) {
  const rota = data.rota!;
  const edit = data.canAssign;
  const name = (login: string) => data.people.find((p) => p.login === login)?.name ?? login;
  const move = (i: number, by: number) => {
    const r = [...rota.rotation];
    [r[i], r[i + by]] = [r[i + by], r[i]];
    save({ rotation: r });
  };
  const outside = data.people.filter((p) => !rota.rotation.includes(p.login));

  return (
    <section className="panel rota" aria-label="Triage rota">
      <div>
        <h2 className="eyebrow">Coming weeks</h2>
        <ol className="rota-weeks">
          {rota.weeks.map((w, n) => (
            <li key={w.start} className={n === 0 ? "now" : undefined}>
              <span className="rota-when">{n === 0 ? "This week" : `Week of ${w.label}`}</span>
              {edit ? (
                <select
                  aria-label={`Triage lead for the week of ${w.label}`}
                  value={w.named ? (w.login ?? "") : ""}
                  onChange={(e) => save({ weeks: { [w.start]: e.target.value } })}
                >
                  <option value="">{w.named || !w.login ? "By the rotation" : `${w.name} (rotation)`}</option>
                  {data.people.map((p) => (
                    <option key={p.login} value={p.login}>
                      {p.name}
                    </option>
                  ))}
                </select>
              ) : (
                <span className="rota-name">{w.name ?? "Nobody yet"}</span>
              )}
              {n === 0 && data.lead?.source === "stand-in" && <span className="rota-sub">{data.lead.name} is standing in</span>}
            </li>
          ))}
        </ol>
      </div>
      <div>
        <h2 className="eyebrow">Rotation</h2>
        {rota.rotation.length === 0 && <p className="empty">No rotation yet. Without one, the lead is whoever was last named.</p>}
        <ol className="rota-order">
          {rota.rotation.map((login, i) => (
            <li key={login}>
              <span className="num">{i + 1}</span>
              <span className="rota-name">{name(login)}</span>
              {edit && (
                <span className="rota-tools">
                  <button type="button" className="quiet" aria-label={`Move ${name(login)} earlier`} disabled={i === 0} onClick={() => move(i, -1)}>
                    ↑
                  </button>
                  <button
                    type="button"
                    className="quiet"
                    aria-label={`Move ${name(login)} later`}
                    disabled={i === rota.rotation.length - 1}
                    onClick={() => move(i, 1)}
                  >
                    ↓
                  </button>
                  <button
                    type="button"
                    className="quiet"
                    aria-label={`Take ${name(login)} out of the rotation`}
                    onClick={() => save({ rotation: rota.rotation.filter((l) => l !== login) })}
                  >
                    Remove
                  </button>
                </span>
              )}
            </li>
          ))}
        </ol>
        {edit && outside.length > 0 && (
          <select aria-label="Add someone to the rotation" value="" onChange={(e) => e.target.value && save({ rotation: [...rota.rotation, e.target.value] })}>
            <option value="">Add someone</option>
            {outside.map((p) => (
              <option key={p.login} value={p.login}>
                {p.name}
              </option>
            ))}
          </select>
        )}
        <p className="fine">
          Each person takes a week, Monday to Sunday. Give a week to someone above to cover leave; use Stand-in to swap for the rest of this week.
        </p>
      </div>
    </section>
  );
}

type Shared = {
  ws: Workspace;
  data: Inbox;
  cos: string[];
  queue: Queue;
  reload: () => void;
  open: number | undefined;
  setOpen: (id: number | null) => void;
};

/** The choices that queue an item, as buttons, with what each one would mean. */
function Choices(props: { value: Choice; set: (c: Choice) => void; data: Inbox; ws: Workspace; cos: string[]; ticket: boolean; dated?: string }) {
  const { value: v, set, data, ws } = props;
  const cell: CellOutlook | undefined = data.cells.find((c) => c.id === v.cell);
  const decide = data.canDecide;

  let outcome = "Choose a cell to see when this would start.";
  if (cell) {
    if (v.cos === "quick")
      outcome = `Goes on ${cell.name}'s quick list${cell.quick ? ` with ${cell.quick} other${cell.quick === 1 ? "" : "s"}` : ""}, off the ranked queue. Flagged if nobody takes it within ${data.quickDays} days.`;
    else if (v.cos === "expedite") outcome = `Goes ahead of everything in ${cell.name}. Someone should pick it up now.`;
    else if (v.top) outcome = `Front of the ${cell.name} queue. ${cell.front || "No pace measured yet"}.`;
    else outcome = `Number ${cell.queued + 1} in the ${cell.name} queue. ${cell.back || "No pace measured yet"}.`;
    if (v.cos === "fixed-date") outcome += " The Floor will warn if that place would miss the date.";
  }

  return (
    <div className="choices-grid">
      <Group name="Cell">
        {data.cells.map((c) => (
          <Pick key={c.id} on={v.cell === c.id} onClick={() => set({ ...v, cell: c.id })}>
            {c.name}
          </Pick>
        ))}
      </Group>
      <Group name="Type">
        {props.ticket ? (
          <span className="fixed">
            <TypeIcon icon="bolt" />
            Unplanned: it arrived as a ticket
          </span>
        ) : (
          ws.kinds.map((k) => (
            <Pick key={k.id} on={v.kind === k.id} onClick={() => set({ ...v, kind: v.kind === k.id ? "" : k.id })}>
              <TypeIcon icon={k.icon} />
              {k.name}
            </Pick>
          ))
        )}
      </Group>
      <Group name="Handling">
        {props.cos.map((c) => (
          <Pick key={c} on={v.cos === c} disabled={c === "expedite" && !decide} onClick={() => set({ ...v, cos: c, top: c === "quick" ? false : v.top })}>
            {label(c)}
          </Pick>
        ))}
      </Group>
      {v.cos === "fixed-date" && (
        <Group name="Due">
          {props.dated ? (
            <span className="fixed">{props.dated}, from the item</span>
          ) : (
            <input type="date" aria-label="Due date" value={v.due} onChange={(e) => set({ ...v, due: e.target.value })} />
          )}
        </Group>
      )}
      {v.cos !== "quick" && v.cos !== "expedite" && (
        <Group name="Place">
          <Pick on={!v.top} onClick={() => set({ ...v, top: false })}>
            Back of the queue
          </Pick>
          <Pick on={v.top} disabled={!decide} onClick={() => set({ ...v, top: true })}>
            Front of the queue
          </Pick>
        </Group>
      )}
      <p className="outcome">{outcome}</p>
      {!decide && (
        <p className="outcome-note">
          Expediting and the front of the queue are for {data.lead ? `the triage lead (${data.lead.name})` : "the triage lead"} or a manager.
        </p>
      )}
    </div>
  );
}

function Group({ name, children }: { name: string; children: React.ReactNode }) {
  return (
    <>
      <span className="group-name">{name}</span>
      <div className="group" role="group" aria-label={name}>
        {children}
      </div>
    </>
  );
}

function Pick(props: { on: boolean; disabled?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" className={props.on ? "pick on" : "pick"} aria-pressed={props.on} disabled={props.disabled} onClick={props.onClick}>
      {props.children}
    </button>
  );
}

/** One item. Closed it is a line; open it shows the choices and what they mean. */
function TriageRow({ row, ws, data, cos, queue, reload, open, setOpen }: Shared & { row: Row }) {
  const c = row.card;
  const decide = data.canDecide;
  const [choice, setChoice] = useState<Choice>({
    cell: c.cell ?? "",
    cos: row.urgent && decide ? "expedite" : "standard",
    kind: c.kind ?? "",
    top: row.urgent && decide,
    due: "",
  });
  const [busy, setBusy] = useState(false);
  const isOpen = open === c.id;

  return (
    <article className={`sort${isOpen ? " open" : ""}${row.urgent ? " is-urgent" : ""}`}>
      <button type="button" className="sort-head" aria-expanded={isOpen} onClick={() => setOpen(isOpen ? null : c.id)}>
        <TypeIcon icon={c.icon} name={c.typeName} />
        <span className="sort-title">{c.title}</span>
        <span className={row.urgent ? "sort-why stop" : "sort-why"}>{row.why}</span>
        <span className={row.late ? "sort-wait warn" : "sort-wait"}>Waiting {row.waiting}</span>
      </button>
      {isOpen && (
        <div className="sort-body">
          {data.canTriage ? (
            <>
              <Choices value={choice} set={setChoice} data={data} ws={ws} cos={cos} ticket={c.mirrored} dated={c.due} />
              <p className="sort-actions">
                <button
                  type="button"
                  disabled={busy || !choice.cell || (choice.cos === "fixed-date" && !c.due && !choice.due)}
                  onClick={async () => {
                    setBusy(true);
                    await queue(c.id, choice);
                    setBusy(false);
                    setOpen(null);
                    reload();
                  }}
                >
                  Queue it
                </button>
                {c.url && (
                  <a className="more" href={c.url} target="_blank" rel="noreferrer">
                    {c.mirrored ? "Open the ticket" : "Open the issue"}
                  </a>
                )}
              </p>
            </>
          ) : (
            <p className="empty">You can see the triage queue but not change it.</p>
          )}
        </div>
      )}
    </article>
  );
}

/** Stack a whole batch of planned work in one go, keeping its order. */
function Batch({ batch, ws, data, cos, queue, reload }: Shared & { batch: Row[] }) {
  const first = batch[0].card;
  const [choice, setChoice] = useState<Choice>({ cell: first.cell ?? "", cos: "standard", kind: first.kind ?? "", top: false, due: "" });
  const [show, setShow] = useState(false);
  const [done, setDone] = useState<number | null>(null);

  const run = async () => {
    // Each item goes to the same end of the queue, so the front is filled in reverse to keep the order.
    const order = choice.top ? [...batch].reverse() : batch;
    let n = 0;
    setDone(0);
    for (const r of order) {
      if (!(await queue(r.card.id, { ...choice, kind: choice.kind || (r.card.kind ?? "") }))) break;
      setDone(++n);
    }
    setDone(null);
    reload();
  };

  if (!show)
    return (
      <p className="batch-offer">
        <button type="button" className="quiet" onClick={() => setShow(true)}>
          Queue all {batch.length} together
        </button>
      </p>
    );
  return (
    <div className="sort open batch">
      <div className="sort-body">
        <Choices value={choice} set={setChoice} data={data} ws={ws} cos={cos.filter((c) => c !== "expedite")} ticket={false} />
        <p className="sort-actions">
          <button type="button" disabled={done !== null || !choice.cell || (choice.cos === "fixed-date" && !choice.due)} onClick={run}>
            {done !== null ? `Queued ${done} of ${batch.length}` : `Queue all ${batch.length}, in this order`}
          </button>
          <button type="button" className="quiet" onClick={() => setShow(false)}>
            Cancel
          </button>
        </p>
      </div>
    </div>
  );
}
