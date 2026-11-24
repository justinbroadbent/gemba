import { useState } from "react";
import { type Review, type Workspace, useData } from "../api";
import { type Go } from "../App";
import { TypeIcon } from "../TypeIcon";
import { useDetail } from "../Detail";

const PERIODS = [
  { days: 30, label: "30 days" },
  { days: 91, label: "Quarter" },
  { days: 182, label: "Half year" },
  { days: 365, label: "Year" },
];
const LISTED = 25; // finished items shown before the rest are folded away

/**
 * A look back: what was finished, where the effort went, how long things took.
 * It states facts and links to them. It scores nobody and compares nobody.
 */
export function ReviewPage({ ws, go }: { ws: Workspace; go: Go }) {
  const params = new URLSearchParams(window.location.search);
  const as = params.get("as") ?? "";
  const days = Number(params.get("days")) || 91;
  const { data, error } = useData<Review>(`/api/w/${ws.slug}/review?days=${days}${as ? `&as=${encodeURIComponent(as)}` : ""}`);
  const [all, setAll] = useState(false);
  const [copied, setCopied] = useState(false);
  const open = useDetail();

  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;

  const show = (nextAs: string, nextDays: number) => {
    const q = new URLSearchParams({ days: String(nextDays) });
    if (nextAs) q.set("as", nextAs);
    go(`/w/${ws.slug}/review?${q}`);
  };
  const unplanned = data.mix.find((s) => s.id === "unplanned");
  const dated = data.dates.met + data.dates.missed;
  const peak = Math.max(1, ...data.weeks.map((w) => w.planned + w.unplanned));
  const items = all ? data.items : data.items.slice(0, LISTED);
  const who = data.person?.name ?? ws.name;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(asText(data, who));
      setCopied(true);
      setTimeout(() => setCopied(false), 2500);
    } catch {
      /* the clipboard can be refused; the page itself is still there to share */
    }
  };

  return (
    <div className="review">
      <header className="page-head with-tools">
        <div>
          <h1>{data.person ? data.person.name : "Looking back"}</h1>
          <p>
            {data.from} to {data.to}. Taken from the work items themselves; open any of them to see the detail.
          </p>
        </div>
        <div className="review-tools">
          <select aria-label="Whose work" value={as} onChange={(e) => show(e.target.value, days)}>
            <option value="">The whole team</option>
            {data.people.map((p) => (
              <option key={p.login} value={p.login}>
                {p.name}
              </option>
            ))}
          </select>
          <div className="chips" role="group" aria-label="Period">
            {PERIODS.map((p) => (
              <button key={p.days} type="button" className={p.days === days ? "chip on" : "chip"} onClick={() => show(as, p.days)}>
                {p.label}
              </button>
            ))}
          </div>
          <button type="button" className="quiet" onClick={copy}>
            {copied ? "Copied" : "Copy as text"}
          </button>
        </div>
      </header>

      {data.finished === 0 ? (
        <p className="empty big">Nothing was finished in this period.</p>
      ) : (
        <>
          <dl className="tiles">
            <div>
              <dt>Finished</dt>
              <dd>
                <b>{data.finished}</b> items
              </dd>
            </div>
            <div>
              <dt>Pace</dt>
              <dd>
                <b>{data.perWeek.toFixed(1)}</b> a week
              </dd>
            </div>
            <div>
              <dt>Unplanned</dt>
              <dd>
                <b>{Math.round(unplanned?.pct ?? 0)}%</b> of what was finished
              </dd>
            </div>
            {dated > 0 && (
              <div>
                <dt>Dates</dt>
                <dd>
                  <b>
                    {data.dates.met} of {dated}
                  </b>{" "}
                  met
                </dd>
              </div>
            )}
          </dl>

          <section className="panel">
            <h2 className="eyebrow">
              Finished each week
              <span className="legend">
                <i className="key planned" /> Planned <i className="key unplanned" /> Unplanned
              </span>
            </h2>
            <div className="weeks" role="img" aria-label="Items finished each week, planned and unplanned">
              {data.weeks.map((w, n) => (
                <div key={n} className="week" title={`Week of ${w.start}: ${w.planned} planned, ${w.unplanned} unplanned`}>
                  <span className="week-n">{w.planned + w.unplanned || ""}</span>
                  {w.unplanned > 0 && <i className="unplanned" style={{ height: `${(w.unplanned / peak) * 100}%` }} />}
                  {w.planned > 0 && <i className="planned" style={{ height: `${(w.planned / peak) * 100}%` }} />}
                </div>
              ))}
            </div>
            <p className="weeks-axis">
              <span>{data.weeks[0]?.start}</span>
              <span>{data.weeks[data.weeks.length - 1]?.start}</span>
            </p>
          </section>

          <div className="review-two">
            <section className="panel">
              <h2 className="eyebrow">Where the work went</h2>
              <Bars rows={data.mix} total={data.finished} />
              {data.cells.length > 1 && (
                <>
                  <h2 className="eyebrow second">By cell</h2>
                  <Bars rows={data.cells} total={data.finished} />
                </>
              )}
            </section>

            <section className="panel">
              <h2 className="eyebrow">How long work took</h2>
              {data.pace.length === 0 ? (
                <p className="empty">Too little finished work was timed to say.</p>
              ) : (
                <table className="took">
                  <thead>
                    <tr>
                      <th>Type</th>
                      <th>Timed</th>
                      <th>Half within</th>
                      <th>Most within</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.pace.map((p) => (
                      <tr key={p.type}>
                        <td>
                          <TypeIcon icon={p.icon} />
                          {p.name}
                        </td>
                        <td>{p.count}</td>
                        <td>{p.typical}</td>
                        <td>{p.most}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              <p className="fine">From start to finish. "Most" is 85 in every 100. Quick tickets that were never queued are not timed.</p>
            </section>
          </div>

          {data.projects.length > 0 && (
            <section className="panel">
              <h2 className="eyebrow">Projects moved forward</h2>
              <ul className="rows">
                {data.projects.map((p) => (
                  <li key={p.name}>
                    <span className="rows-title">
                      <TypeIcon icon={p.icon} />
                      {p.url ? (
                        <a href={p.url} target="_blank" rel="noreferrer">
                          {p.name}
                        </a>
                      ) : (
                        p.name
                      )}
                    </span>
                    <span>
                      <b>{p.finished}</b> finished in this period
                    </span>
                    <span className="rows-note">{p.remaining === 0 ? "Project complete" : `${p.remaining} of ${p.total} still to do`}</span>
                  </li>
                ))}
              </ul>
            </section>
          )}

          {data.notable.length > 0 && (
            <section className="panel">
              <h2 className="eyebrow">Worth a mention</h2>
              <ul className="rows">
                {data.notable.map((d) => (
                  <li key={d.card.id}>
                    <button type="button" className="rows-title" onClick={() => open(d.card)}>
                      <TypeIcon icon={d.card.icon} name={d.card.typeName} />
                      {d.card.title}
                    </button>
                    <span>{d.note}</span>
                    <span className="rows-note">
                      {d.took ? `Took ${d.took}, finished` : "Finished"} {d.closed}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          )}

          {data.person && (
            <section className="panel">
              <h2 className="eyebrow">
                Everything finished <span className="count">{data.items.length}</span>
              </h2>
              <ul className="rows">
                {items.map((d) => (
                  <li key={d.card.id}>
                    <button type="button" className="rows-title" onClick={() => open(d.card)}>
                      <TypeIcon icon={d.card.icon} name={d.card.typeName} />
                      {d.card.title}
                    </button>
                    <span>{d.card.project ?? ""}</span>
                    <span className="rows-note">
                      {d.took ? `Took ${d.took}, finished` : "Finished"} {d.closed}
                    </span>
                  </li>
                ))}
              </ul>
              {data.items.length > LISTED && (
                <button type="button" className="quiet" onClick={() => setAll(!all)}>
                  {all ? "Show fewer" : `Show all ${data.items.length}`}
                </button>
              )}
            </section>
          )}
        </>
      )}
    </div>
  );
}

function Bars({ rows, total }: { rows: Review["mix"]; total: number }) {
  return (
    <div className="mix flat">
      {rows.map((s) => (
        <div key={s.id} className="mix-row" title={`${s.count} of ${total} items`}>
          <span className="mix-name">
            {s.icon && <TypeIcon icon={s.icon} />}
            {s.name}
          </span>
          <span className="mix-bar">
            <i className={s.id === "unplanned" ? "unplanned" : undefined} style={{ width: `${s.pct}%` }} />
          </span>
          <span className="mix-pct">{s.count}</span>
        </div>
      ))}
    </div>
  );
}

/** The same facts as plain text, for pasting into a report or a review. */
function asText(r: Review, who: string) {
  const lines = [`${who}: ${r.from} to ${r.to}`, "", `Finished ${r.finished} items, about ${r.perWeek.toFixed(1)} a week.`];
  if (r.dates.met + r.dates.missed > 0) lines.push(`Dates met: ${r.dates.met} of ${r.dates.met + r.dates.missed}.`);
  lines.push("", "Where the work went:");
  for (const s of r.mix) lines.push(`- ${s.name}: ${s.count} (${Math.round(s.pct)}%)`);
  if (r.pace.length) {
    lines.push("", "How long work took, start to finish:");
    for (const p of r.pace) lines.push(`- ${p.name}: half within ${p.typical}, most within ${p.most} (${p.count} timed)`);
  }
  if (r.projects.length) {
    lines.push("", "Projects moved forward:");
    for (const p of r.projects)
      lines.push(`- ${p.name}: ${p.finished} finished, ${p.remaining === 0 ? "project complete" : `${p.remaining} of ${p.total} still to do`}`);
  }
  if (r.notable.length) {
    lines.push("", "Worth a mention:");
    for (const d of r.notable) lines.push(`- ${d.card.title} (${d.note?.toLowerCase()}, finished ${d.closed})${d.card.url ? ` ${d.card.url}` : ""}`);
  }
  if (r.items.length) {
    lines.push("", "Everything finished:");
    for (const d of r.items) lines.push(`- ${d.closed}: ${d.card.title} [${d.card.typeName}]${d.card.url ? ` ${d.card.url}` : ""}`);
  }
  return lines.join("\n");
}
