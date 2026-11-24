import { useEffect } from "react";
import { type Card, type Floor, type Mine, type Workspace, useData } from "../api";
import { type Go, Link } from "../App";
import { TimeBar, TypeMark, useDetail } from "../Detail";

/**
 * The floor, one person at a time, for a standup. Whoever is running the
 * meeting steps through the team; each step shows what that person has in
 * hand, what they finished since the last standup, and anything in their way.
 * Anyone can open it. It changes nothing.
 */
export function Standup({ ws, go, floor }: { ws: Workspace; go: Go; floor: Floor }) {
  const people = floor.lanes.flatMap((l) => l.hands.map((h) => ({ login: h.login, name: h.name, cell: l.cell.name })));
  const at = new URLSearchParams(window.location.search).get("at") ?? people[0]?.login ?? "";
  const n = Math.max(0, people.findIndex((p) => p.login === at));
  const person = people[n];
  const { data } = useData<Mine>(person ? `/api/w/${ws.slug}/mine?as=${encodeURIComponent(person.login)}` : null);
  const open = useDetail();

  const to = (i: number) => `/w/${ws.slug}/floor?standup=1&at=${encodeURIComponent(people[i].login)}`;
  const prev = n > 0 ? to(n - 1) : null;
  const next = n < people.length - 1 ? to(n + 1) : null;
  const done = `/w/${ws.slug}/floor`;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest("input, select, textarea")) return;
      if (e.key === "ArrowRight" || e.key === " ") {
        e.preventDefault();
        go(next ?? done);
      }
      if (e.key === "ArrowLeft" && prev) go(prev);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [go, next, prev, done]);

  if (!person) return <p className="empty big">There is nobody on this floor yet.</p>;

  const blocked = data?.doing.filter((c) => c.blocked) ?? [];
  const line = (c: Card, note: string, tone = "") => (
    <li key={c.id}>
      <button type="button" className="standup-item" onClick={() => open(c)}>
        <span className="standup-title">
          <TypeMark card={c} />
          {c.title}
        </span>
        <span className={`standup-note ${tone}`}>{note}</span>
        <TimeBar card={c} />
      </button>
    </li>
  );

  return (
    <div className="standup">
      <nav className="standup-who" aria-label="The team">
        <p className="eyebrow">
          Standup
          <Link to={done} go={go} className="more">
            Leave
          </Link>
        </p>
        <ol>
          {people.map((p, i) => (
            <li key={p.login}>
              <Link to={to(i)} go={go} className={i === n ? "on" : i < n ? "seen" : undefined}>
                <span>{p.name}</span>
                <span className="standup-cell">{p.cell}</span>
              </Link>
            </li>
          ))}
        </ol>
      </nav>

      <section className="standup-main" key={person.login}>
        <header>
          <p className="nafuda">{person.cell}</p>
          <h1>{person.name}</h1>
          <p className="lede">
            {n + 1} of {people.length}
          </p>
        </header>

        {!data ? null : (
          <>
            {blocked.length > 0 && (
              <div className="standup-block stop">
                <h2 className="eyebrow">In the way</h2>
                <ul>{blocked.map((c) => line(c, `Blocked ${c.blockedFor ?? ""}: ${c.blocked}`, "stop"))}</ul>
              </div>
            )}

            <div className="standup-block">
              <h2 className="eyebrow">
                In hand
                <span className={`count${data.doing.length > (data.person?.limit ?? 0) ? " warn" : ""}`}>
                  {data.doing.length} of {data.person?.limit}
                </span>
              </h2>
              {data.doing.length === 0 ? (
                <p className="empty">Nothing in hand.</p>
              ) : (
                <ul>
                  {data.doing.map((c) =>
                    line(
                      c,
                      c.blocked
                        ? `Blocked ${c.blockedFor ?? ""}`
                        : c.quietFor
                          ? `No activity for ${c.quietFor}`
                          : `Day ${c.day ?? 1}${c.usualDays ? ` of a usual ${Math.round(c.usualDays)}` : ""}`,
                      c.blocked ? "stop" : c.quietFor || c.agingFor ? "warn" : "",
                    ),
                  )}
                </ul>
              )}
            </div>

            <div className="standup-block">
              <h2 className="eyebrow">
                Finished since the last standup <span className="count">{data.done.length}</span>
              </h2>
              {data.done.length === 0 ? <p className="empty">Nothing finished.</p> : <ul>{data.done.map((c) => line(c, c.typeName))}</ul>}
            </div>

            {data.waiting.length > 0 && (
              <div className="standup-block">
                <h2 className="eyebrow">Waiting on others</h2>
                <ul>{data.waiting.map((c) => line(c, `${c.waitingFor ?? ""} on ${c.waitingOn}${c.chase ? ". Time to chase" : ""}`, c.chase ? "warn" : ""))}</ul>
              </div>
            )}

            {data.next[0] && (
              <div className="standup-block">
                <h2 className="eyebrow">{data.full ? "Next, when a hand is free" : "Next to pull"}</h2>
                <ul>{line(data.next[0].card, data.next[0].card.risk ?? data.next[0].card.reason ?? "", data.next[0].card.risk ? "stop" : "")}</ul>
              </div>
            )}
          </>
        )}

        <p className="standup-step">
          {prev ? (
            <Link to={prev} go={go} className="button quiet-button">
              Back
            </Link>
          ) : (
            <span />
          )}
          <Link to={next ?? done} go={go} className="button">
            {next ? `Next: ${people[n + 1].name}` : "Finish"}
          </Link>
        </p>
      </section>
    </div>
  );
}
