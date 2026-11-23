import { createContext, useContext, useEffect, type ReactNode } from "react";
import { type Card, home, label } from "./api";

/** Any item anywhere can be opened in the side panel. */
export const DetailContext = createContext<(card: Card | null) => void>(() => {});
export const useDetail = () => useContext(DetailContext);

/** The mark for a type of work: its initial, with unplanned work in the accent colour. */
export function TypeMark({ card }: { card: Card }) {
  return (
    <span className={`mark${card.type === "unplanned" ? " unplanned" : ""}`} title={card.typeName} aria-label={card.typeName}>
      {card.typeName.charAt(0)}
    </span>
  );
}

/** How far through its usual time an in-progress item is. */
export function TimeBar({ card }: { card: Card }) {
  if (!card.day || !card.usualDays) return null;
  const pct = Math.min(100, (card.day / card.usualDays) * 100);
  const over = card.day > card.usualDays;
  return (
    <span
      className={`timebar${over ? " over" : ""}`}
      title={`Day ${card.day}. Work like this usually takes up to ${Math.round(card.usualDays)} days.`}
    >
      <i style={{ width: `${pct}%` }} />
    </span>
  );
}

function Row({ name, children }: { name: string; children: ReactNode }) {
  return (
    <>
      <dt>{name}</dt>
      <dd>{children}</dd>
    </>
  );
}

export function Detail({ card, close }: { card: Card; close: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [close]);

  return (
    <>
      <div className="scrim" onClick={close} />
      <aside className="detail" role="dialog" aria-label={card.title}>
        <button type="button" className="quiet close" onClick={close} aria-label="Close">
          Close
        </button>
        <p className="detail-type">
          <TypeMark card={card} /> {card.typeName} · #{card.id}
        </p>
        <h2>{card.title}</h2>

        {card.blocked && (
          <p className="callout stop">
            Blocked{card.blockedFor && ` for ${card.blockedFor}`}: {card.blocked}
          </p>
        )}
        {card.risk && <p className="callout stop">{card.risk}</p>}
        {card.agingFor && (
          <p className="callout warn">
            In progress {card.agingFor}
            {card.usual && `. Work like this usually takes under ${card.usual}`}
          </p>
        )}
        {card.quietFor && !card.agingFor && <p className="callout warn">No activity for {card.quietFor}</p>}

        <dl>
          {card.reason && <Row name="Why here">{card.reason}</Row>}
          {card.starts && <Row name="Outlook">{card.starts}</Row>}
          {card.day && (
            <Row name="In progress">
              Day {card.day}
              {card.usualDays ? ` of a usual ${Math.round(card.usualDays)}` : ""}
            </Row>
          )}
          {card.owner && <Row name="With">{card.owner}</Row>}
          {card.project && (
            <Row name="Project">
              {card.projectUrl ? (
                <a href={card.projectUrl} target="_blank" rel="noreferrer">
                  {card.project}
                </a>
              ) : (
                card.project
              )}
            </Row>
          )}
          {card.cos && <Row name="Handling">{label(card.cos) + (card.due ? `, due ${card.due}` : "")}</Row>}
        </dl>

        {card.url && (
          <a className="button" href={card.url} target="_blank" rel="noreferrer">
            {home(card)}
          </a>
        )}
      </aside>
    </>
  );
}
