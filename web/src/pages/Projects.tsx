import { type Project, type Workspace, useData } from "../api";

/** Projects, rolled up from the items that carry their name. Nothing here is entered by hand. */
export function ProjectsPage({ ws }: { ws: Workspace }) {
  const { data, error } = useData<{ projects: Project[] }>(`/api/w/${ws.slug}/projects`, 60_000);
  if (error) return <p className="notice stop">{error}</p>;
  if (!data) return null;
  return (
    <>
      <header className="page-head">
        <h1>Projects</h1>
        <p>Progress and outlook for each project, worked out from its items. The forecast uses the recent pace, not estimates.</p>
      </header>
      {data.projects.length === 0 && <p className="empty big">No items carry a project yet.</p>}
      <div className="projects">
        {data.projects.map((p) => (
          <article key={p.name} className={`project${p.stalled ? " is-blocked" : ""}`}>
            <p className="eyebrow">{p.kind || "Project"}</p>
            <h2>
              {p.url ? (
                <a href={p.url} target="_blank" rel="noreferrer">
                  {p.name}
                </a>
              ) : (
                p.name
              )}
            </h2>
            <div className="progress" title={`${p.done} of ${p.total} items finished`}>
              <i style={{ width: `${p.total ? (p.done / p.total) * 100 : 0}%` }} />
            </div>
            <p className="project-count">
              <b>{p.done}</b> of {p.total} finished
            </p>
            <p className="card-meta">
              <span>{p.doing} in progress</span>
              <span>{p.queued} queued</span>
              {p.blocked > 0 && <span className="flag stop">{p.blocked} blocked</span>}
              {p.cells.length > 0 && <span>{p.cells.join(", ")}</span>}
            </p>
            {p.stalled ? (
              <p className="card-alert stop">Nothing in progress, and nothing has moved for {p.quiet}</p>
            ) : (
              p.forecast && <p className="project-forecast">{p.forecast}</p>
            )}
          </article>
        ))}
      </div>
    </>
  );
}
