import { type Card, label } from "./api";

/** One work item. Flags appear only when they call for attention. */
export function CardView({ card, owner = true, dim = false }: { card: Card; owner?: boolean; dim?: boolean }) {
  const cls = ["card", card.blocked && "is-blocked", card.cos === "expedite" && "is-expedite", dim && "is-dim"]
    .filter(Boolean)
    .join(" ");
  return (
    <article className={cls}>
      <p className="card-title">
        {card.url ? (
          <a href={card.url} target="_blank" rel="noreferrer">
            {card.title}
          </a>
        ) : (
          card.title
        )}
      </p>
      <p className="card-meta">
        <span className="num">#{card.id}</span>
        {owner && card.owner && <span>{card.owner}</span>}
        {card.cos === "expedite" && <span className="flag stop">特急 Expedite</span>}
        {card.cos === "fixed-date" && <span className="flag">Due {card.due ?? "date not set"}</span>}
        {card.cos === "background" && <span>{label(card.cos)}</span>}
        {card.mirrored && <span>Ticket</span>}
      </p>
      {card.blocked && (
        <p className="card-alert stop">
          <span aria-hidden="true">待ち </span>
          {card.blocked}
          {card.blockedFor && ` · ${card.blockedFor}`}
        </p>
      )}
      {card.agingFor && <p className="card-alert warn">In progress {card.agingFor}</p>}
    </article>
  );
}
