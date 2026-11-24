import { useEffect, useRef, useState } from "react";
import { type Card, type Mine, type Workspace, ApiError, home, post, useData } from "../api";
import { TimeBar, TypeMark, useDetail } from "../Detail";

/** One person's answer to "what should I be doing?": what is in hand, and the one thing to pull next. */
export function NowPage({ ws }: { ws: Workspace }) {
  const remembered = `gemba:${ws.slug}:me`;
  // A link can name the person (?as=login); otherwise it is whoever signed in, or whoever was picked last.
  const [as, setAs] = useState(() => new URLSearchParams(window.location.search).get("as") ?? ws.me ?? read(remembered) ?? "");
  const { data, error, reload } = useData<Mine>(`/api/w/${ws.slug}/mine${as ? `?as=${encodeURIComponent(as)}` : ""}`, 30_000);
  const open = useDetail();
  const changes = useChanges(ws.slug, data);
  const [notice, setNotice] = useState<string | null>(null);
  const act = async (path: string, body: unknown) => {
    try {
      await post(`/api/w/${ws.slug}/items/${path}`, body);
      setNotice(null);
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Could not save that.");
    }
    reload();
  };

  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;

  const choose = (login: string) => {
    write(remembered, login);
    setAs(login);
  };
  const picker = (
    <select aria-label="Whose work to show" value={data.person?.login ?? ""} onChange={(e) => choose(e.target.value)}>
      <option value="" disabled>
        Choose a person
      </option>
      {data.people.map((p) => (
        <option key={p.login} value={p.login}>
          {p.name}
        </option>
      ))}
    </select>
  );

  if (!data.person)
    return (
      <section className="now-empty">
        <h1>Whose work?</h1>
        <p className="lede">Pick yourself once and this page opens on your work from then on.</p>
        {picker}
      </section>
    );

  const free = Math.max(0, data.person.limit - data.doing.length);
  const [first, ...rest] = data.next;

  return (
    <div className="now">
      <div className="now-main">
        <header className="now-head">
          <h1>{data.person.name}</h1>
          {picker}
        </header>

        {data.lead && (
          <p className="callout duty">
            You are the triage lead this week.{" "}
            <a href={`/w/${ws.slug}/triage`}>{data.triage === 0 ? "Nothing is waiting." : `${data.triage} waiting for a decision.`}</a>
          </p>
        )}

        <section>
          <h2 className="eyebrow">
            In your hands
            <span className="count">
              {data.doing.length} of {data.person.limit}
            </span>
          </h2>
          {notice && <p className="notice stop">{notice}</p>}
          <div className="hands">
            {data.doing.map((c, n) => (
              <div key={c.id} className="hand-cell">
              <button type="button" className={`hand ${c.blocked ? "is-blocked" : ""}`} onClick={() => open(c)}>
                <span className="hand-title">
                  <TypeMark card={c} />
                  {c.title}
                </span>
                <span className={`hand-note${c.blocked ? " stop" : c.agingFor || c.quietFor ? " warn" : ""}`}>
                  {c.blocked
                    ? `Blocked ${c.blockedFor ?? ""}: ${c.blocked}`
                    : n >= data.person!.limit
                      ? "Beyond your limit: finish or hand one back"
                      : c.quietFor
                        ? `No activity for ${c.quietFor}`
                        : `Day ${c.day ?? 1}${c.usualDays ? ` of a usual ${Math.round(c.usualDays)}` : ""}`}
                </span>
                <TimeBar card={c} />
              </button>
              {data.canAct && !c.blocked && (c.mirrored ? null : <Park onSave={(on) => act(`${c.id}/wait`, { on })} />)}
              </div>
            ))}
            {Array.from({ length: free }, (_, n) => (
              <span key={n} className="hand free">
                Free hand
              </span>
            ))}
          </div>
        </section>

        {data.waiting.length > 0 && (
          <section>
            <h2 className="eyebrow">
              Waiting on others
              <span className={`count${data.waiting.length > data.waitLimit ? " warn" : ""}`}>
                {data.waiting.length} of {data.waitLimit}. These do not hold a hand.
              </span>
            </h2>
            <ol className="after">
              {data.waiting.map((c) => (
                <li key={c.id} className="waiting-row">
                  <button type="button" onClick={() => open(c)}>
                    <span className="num" />
                    <span className="after-title">
                      <TypeMark card={c} />
                      {c.title}
                    </span>
                    <span className={`after-why${c.chase ? " warn" : ""}`}>
                      {c.waitingFor} on {c.waitingOn}
                      {c.chase && ". Time to chase"}
                    </span>
                  </button>
                  {data.canAct && !c.mirrored && (
                    <button type="button" className="quiet" onClick={() => act(`${c.id}/resume`, {})}>
                      Take it back
                    </button>
                  )}
                </li>
              ))}
            </ol>
          </section>
        )}

        <section>
          <h2 className="eyebrow">{data.full ? "When a hand is free" : "Pull next"}</h2>
          {!first ? (
            <p className="empty">Nothing is waiting for you in the {data.cell} queue.</p>
          ) : (
            <article className={`next ${data.full ? "waiting" : ""}`}>
              {changes.get(first.card.id) && <p className="fresh">{changes.get(first.card.id)}</p>}
              <h3>
                <TypeMark card={first.card} />
                {first.card.title}
              </h3>
              <p className="next-why">{first.card.reason}</p>
              {first.card.risk && <p className="callout stop">{first.card.risk}</p>}
              <p className="next-meta">
                {data.full ? "Your hands are full. Finish or hand one back before pulling this." : first.card.starts}
              </p>
              <p className="next-actions">
                {data.canTake && !first.card.mirrored && (
                  <button type="button" onClick={() => act(`${first.card.id}/take`, { as: data.person!.login })}>
                    {data.full ? "Take it anyway" : "Take it"}
                  </button>
                )}
                {first.card.url && (
                  <a className={data.canTake && !first.card.mirrored ? "more" : "button"} href={first.card.url} target="_blank" rel="noreferrer">
                    {home(first.card)}
                  </a>
                )}
                <button type="button" className="quiet" onClick={() => open(first.card)}>
                  Details
                </button>
              </p>
            </article>
          )}
        </section>

        {data.quick.length > 0 && (
          <section className="quick-work">
            <h2>
              Quick work <span className="quick-count">{data.quick.length}</span>
            </h2>
            <p className="quick-lede">
              Small jobs waiting in {data.cell}. Take the oldest when you have a free hand but not the time for something bigger.
            </p>
            <ol className="after">
              {data.quick.map((card) => (
                <li key={card.id} className="waiting-row">
                  <button type="button" onClick={() => open(card)}>
                    <span className="num" />
                    <span className="after-title">
                      <TypeMark card={card} />
                      {card.title}
                    </span>
                    <span className={`after-why${card.risk ? " warn" : ""}`}>{card.risk ?? card.reason}</span>
                  </button>
                  {data.canTake && !card.mirrored && (
                    <button type="button" className="quiet" onClick={() => act(`${card.id}/take`, { as: data.person!.login })}>
                      Take it
                    </button>
                  )}
                </li>
              ))}
            </ol>
          </section>
        )}

        {rest.length > 0 && (
          <section>
            <h2 className="eyebrow">After that</h2>
            <ol className="after">
              {rest.map(({ card, position }) => (
                <li key={card.id}>
                  <button type="button" onClick={() => open(card)}>
                    <span className="num">{position}</span>
                    <span className="after-title">
                      <TypeMark card={card} />
                      {card.title}
                      {changes.get(card.id) && <em className="fresh">{changes.get(card.id)}</em>}
                    </span>
                    <span className={`after-why${card.risk ? " stop" : ""}`}>{card.risk ?? card.reason}</span>
                  </button>
                </li>
              ))}
            </ol>
          </section>
        )}

      </div>

      {data.priorities.length > 0 && (
        <aside className="scroll" aria-label="Current priorities">
          <h2>Priorities</h2>
          <ol>
            {data.priorities.map((line, n) => (
              <li key={n}>{line}</li>
            ))}
          </ol>
          {data.editUrl && (
            <a className="more" href={data.editUrl} target="_blank" rel="noreferrer">
              Edit in GitHub
            </a>
          )}
        </aside>
      )}
    </div>
  );
}

/** Parks an item that cannot move until someone else does their part. */
function Park({ onSave }: { onSave: (on: string) => void }) {
  const [open, setOpen] = useState(false);
  const [on, setOn] = useState("");
  if (!open)
    return (
      <button type="button" className="quiet park" onClick={() => setOpen(true)}>
        Waiting on someone
      </button>
    );
  return (
    <form
      className="park-form"
      onSubmit={(e) => {
        e.preventDefault();
        onSave(on);
        setOpen(false);
      }}
    >
      <input autoFocus required maxLength={120} placeholder="Who or what is it waiting on?" value={on} onChange={(e) => setOn(e.target.value)} />
      <button type="submit">Set aside</button>
      <button type="button" className="quiet" onClick={() => setOpen(false)}>
        Cancel
      </button>
    </form>
  );
}

/**
 * Marks what changed in a person's list since they last looked. Boards never
 * say what moved; this does, using only this browser's memory of the last visit.
 */
function useChanges(slug: string, data: Mine | null) {
  const before = useRef<number[] | null>(null);
  const [changes, setChanges] = useState(new Map<number, string>());
  const login = data?.person?.login;

  useEffect(() => {
    if (!data || !login) return;
    const key = `gemba:${slug}:${login}:next`;
    const now = data.next.map((n) => n.card.id);
    if (before.current === null) {
      try {
        before.current = JSON.parse(read(key) ?? "null") ?? now;
      } catch {
        before.current = now;
      }
      write(key, JSON.stringify(now));
    }
    const was = before.current!;
    const out = new Map<number, string>();
    now.forEach((id, n) => {
      const old = was.indexOf(id);
      if (old < 0) out.set(id, "New since you last looked");
      else if (old > n) out.set(id, "Moved up since you last looked");
    });
    setChanges(out);
  }, [data, login, slug]);

  useEffect(() => {
    before.current = null; // a different person: start again
  }, [login]);

  return changes;
}

function read(key: string) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* private windows may refuse; the page works without it */
  }
}

export type { Card };
