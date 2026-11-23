import { type Card } from "./api";
import { TimeBar, TypeMark, useDetail } from "./Detail";

/** One work item as a row. Flags appear only when they call for attention. Click for the full picture. */
export function CardView({
  card,
  owner = true,
  dim = false,
  plain = false,
}: {
  card: Card;
  owner?: boolean;
  dim?: boolean;
  plain?: boolean; // not clickable: for use inside another control
}) {
  const open = useDetail();
  const cls = ["card", !plain && "opens", card.blocked && "is-blocked", card.cos === "expedite" && "is-expedite", dim && "is-dim"]
    .filter(Boolean)
    .join(" ");
  return (
    <article className={cls} onClick={plain ? undefined : () => open(card)} data-item={card.id}>
      <p className="card-title">
        <TypeMark card={card} />
        {card.title}
      </p>
      <p className="card-meta">
        <span className="num">#{card.id}</span>
        {owner && card.owner && <span>{card.owner}</span>}
        {card.project && <span>{card.project}</span>}
        {card.cos === "expedite" && <span className="flag stop">Expedite</span>}
        {card.cos === "fixed-date" && <span className="flag">Due {card.due ?? "date not set"}</span>}
        {card.starts && <span>{card.starts}</span>}
      </p>
      {card.risk && <p className="card-alert stop">{card.risk}</p>}
      {card.blocked && (
        <p className="card-alert stop">
          {card.blocked}
          {card.blockedFor && ` · ${card.blockedFor}`}
        </p>
      )}
      {card.agingFor && <p className="card-alert warn">In progress {card.agingFor}</p>}
      <TimeBar card={card} />
    </article>
  );
}
