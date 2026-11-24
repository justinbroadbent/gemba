import { useCallback, useEffect, useRef, useState } from "react";

// The API returns view models that are ready to render. No flow rules live here.

export type Cell = { id: string; name: string };
export type Kind = { id: string; name: string; project: boolean; icon: string };

export type Card = {
  id: number;
  title: string;
  url?: string;
  owner?: string;
  cell?: string;
  type: string; // "unplanned", a kind id, or "other"
  typeName: string;
  icon: string;
  kind?: string;
  project?: string;
  projectUrl?: string;
  cos?: string;
  due?: string;
  mirrored: boolean;
  blocked?: string;
  blockedFor?: string;
  agingFor?: string;
  usual?: string;
  day?: number;
  usualDays?: number;
  waitingOn?: string;
  waitingFor?: string;
  chase?: boolean;
  quietFor?: string;
  reason?: string;
  starts?: string;
  waitDays?: number;
  risk?: string;
};

export type Hand = { login: string; name: string; limit: number; items: Card[]; waiting: Card[]; pull: Card | null };
export type Share = { id: string; name: string; icon: string; count: number; pct: number };
export type Alert = { level: "stop" | "warn"; text: string; cell?: string; id?: number; url?: string };

export type Lane = {
  cell: Cell;
  throughput: number;
  quickRate: number;
  measured: boolean;
  line: number;
  wipLimit: number;
  mix: Share[];
  hands: Hand[];
  freeHands: number;
  arrivals: number;
  arrivalsUsual: number;
  queue: Card[];
  quick: Card[];
  doing: Card[];
  done: Card[];
  quickDone: number;
};

export type Floor = {
  signals: { lead?: string; triage: number; blocked: number; aging: number; over: number };
  attention: Alert[];
  problems?: string[];
  lanes: Lane[];
};

export type CellOutlook = { id: string; name: string; queued: number; quick: number; free: number; back: string; front: string };
export type Lead = { login: string; name: string; since: string; days: number; source: string };
export type Rota = { rotation: string[]; weeks: { start: string; label: string; login?: string; name?: string; named: boolean }[] };
export type TriageRow = { card: Card; why: string; urgent: boolean; priority?: string; waiting: string; late: boolean };
export type Inbox = {
  fastLane: number;
  targetHours: number;
  quickDays: number;
  late: number;
  rows: TriageRow[];
  fresh: TriageRow[];
  cells: CellOutlook[];
  lead: Lead | null;
  rota: Rota | null;
  people: PersonRef[];
  canTriage: boolean;
  canDecide: boolean;
  canAssign: boolean;
};

export type Candidate = { id: number; title: string; cell?: string; cellName?: string; stage: string; icon: string; typeName: string; project?: string };
export type Preview = {
  item: Card;
  lane: Lane;
  falls: Card | null;
  alreadyAbove: boolean;
  behind: number;
  delay?: string;
  atRisk: Card[];
  pause: { card: Card; note: string }[];
};
export type Tradeoff = { candidates: Candidate[]; preview: Preview | null };
export type Note = { id: number; title: string; text: string };

export type Project = {
  name: string;
  url?: string;
  kind: string;
  total: number;
  done: number;
  doing: number;
  blocked: number;
  queued: number;
  remaining: number;
  cells: string[];
  quiet: string;
  stalled: boolean;
  forecast: string;
};

export type Finished = { card: Card; closed: string; took?: string; note?: string };
export type Review = {
  days: number;
  from: string;
  to: string;
  person: PersonRef | null;
  people: PersonRef[];
  finished: number;
  perWeek: number;
  mix: Share[];
  weeks: { start: string; planned: number; unplanned: number }[];
  pace: { type: string; name: string; icon: string; count: number; typical: string; most: string }[];
  dates: { met: number; missed: number };
  cells: Share[];
  projects: { name: string; url?: string; icon: string; finished: number; remaining: number; total: number }[];
  notable: Finished[];
  items: Finished[];
  holding: Card[];
};

export type PersonRef = { login: string; name: string; count: number; limit: number };
export type Mine = {
  person: PersonRef | null;
  people: PersonRef[];
  full: boolean;
  doing: Card[];
  next: { card: Card; position: number; above: boolean }[];
  quick: Card[];
  waiting: Card[];
  done: Card[];
  waitLimit: number;
  canAct: boolean;
  cell?: string;
  priorities: string[];
  editUrl?: string;
  lead: boolean;
  triage: number;
};

export type Workspace = {
  slug: string;
  name: string;
  canWrite: boolean;
  canTriage: boolean;
  cells: Cell[];
  kinds: Kind[];
  horizonWeeks: number;
  me?: string;
};

export type Session = {
  user: string;
  brand: { name: string; logo?: string };
  theme: { light: Record<string, string>; dark: Record<string, string> };
  cos: string[];
  workspaces: Workspace[];
};

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, init);
  } catch {
    throw new ApiError(0, "Could not reach the server.");
  }
  const body = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError(res.status, body?.error ?? `Request failed (${res.status})`);
  return body as T;
}

export const get = <T,>(path: string) => call<T>(path);

export const post = async <T,>(path: string, body: unknown) => {
  const out = await call<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Gemba": "1" },
    body: JSON.stringify(body),
  });
  // Tell the change watcher this browser made a change, so it is not announced back to its author.
  if (!path.endsWith("/refresh")) window.dispatchEvent(new Event("gemba:wrote"));
  return out;
};

/** Loads a path, reloads on demand, and optionally refreshes while the tab is visible. */
export function useData<T>(path: string | null, pollMs = 0) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const shown = useRef<string | null>(null);
  const reload = useCallback(() => setTick((t) => t + 1), []);

  useEffect(() => {
    if (shown.current !== path) {
      shown.current = path;
      setData(null); // never show one workspace's data under another's name
    }
    if (!path) return;
    let live = true;
    get<T>(path)
      .then((d) => {
        if (live) {
          setData(d);
          setError(null);
        }
      })
      .catch((e: Error) => live && setError(e.message));
    return () => {
      live = false;
    };
  }, [path, tick]);

  useEffect(() => {
    if (!pollMs) return;
    const id = setInterval(() => !document.hidden && reload(), pollMs);
    return () => clearInterval(id);
  }, [pollMs, reload]);

  return { data, error, reload };
}

export const label = (cos: string) => cos.charAt(0).toUpperCase() + cos.slice(1).replace("-", " ");

/** Where an item is worked: tickets in the ticketing system, the rest in GitHub. */
export const home = (c: Card) => (c.mirrored ? "Open the ticket" : "Open the issue");
