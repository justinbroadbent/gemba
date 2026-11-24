import type { ReactNode } from "react";

/**
 * The small marks for types of work, drawn on a 16-unit grid in the current
 * text colour. A kind of work picks one by name in the configuration; a name
 * that is not here falls back to the dot.
 */
const ICONS: Record<string, ReactNode> = {
  // Unplanned: it strikes without warning.
  bolt: <path d="M9.4 1.2 3.2 9.2h3.9l-.6 5.6 6.3-8.2H8.9z" />,
  // Enterprise project: the whole company's building.
  tower: <path d="M4 14.2V2.2h8v12M2 14.2h12M6.6 5.2h2.8M6.6 8h2.8M6.6 10.8h2.8" />,
  // Department project: our own house.
  house: <path d="M2.2 7.6 8 2.4l5.8 5.2M3.8 6.4v7.8h8.4V6.4M6.8 14.2v-3.6h2.4v3.6" />,
  // Maintenance: keeping things running.
  wrench: (
    <g transform="rotate(45 8 8)">
      <path d="M6.3 1.1a3.4 3.4 0 1 0 3.4 0v2.8H6.3z" />
      <path d="M6.7 7.3v6.3a1.3 1.3 0 0 0 2.6 0V7.3" />
    </g>
  ),
  flag: <path d="M4 14.4V2.2M4 2.8h8.4l-2.2 3 2.2 3H4" />,
  shield: <path d="M8 1.8l5 1.8v4.2c0 3.1-2 5.3-5 6.5-3-1.2-5-3.4-5-6.5V3.6z" />,
  dot: <circle cx="8" cy="8" r="2.8" />,
};

export function TypeIcon({ icon, name }: { icon: string; name?: string }) {
  const known = icon in ICONS ? icon : "dot";
  return (
    <span className={`mark ${known}`} title={name} role={name ? "img" : undefined} aria-label={name} aria-hidden={name ? undefined : true}>
      <svg viewBox="0 0 16 16">{ICONS[known]}</svg>
    </span>
  );
}
