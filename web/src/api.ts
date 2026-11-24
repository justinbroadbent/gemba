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
  quietFor?: string;
  reason?: string;
  starts?: string;
  waitDays?: number;
  risk?: string;
};

export type Hand = { login: string; name: string; limit: number; items: Card[]; pull: Card | null };
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
  doing: Card[];
  done: Card[];
  quickDone: number;
};

export type Floor = {
  signals: { triage: number; blocked: number; aging: number; over: number };
  attention: Alert[];
  lanes: Lane[];
};

export type Inbox = {
  fastLane: number;
  rows: { card: Card; why: string; urgent: boolean; priority?: string }[];
};

export type Candidate = { id: number; title: string; cell?: string; stage: string };
export type Preview = { item: Card; lane: Lane; falls: Card | null; alreadyAbove: boolean };
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

export type PersonRef = { login: string; name: string; count: number; limit: number };
export type Mine = {
  person: PersonRef | null;
  people: PersonRef[];
  full: boolean;
  doing: Card[];
  next: { card: Card; position: number; above: boolean }[];
  cell?: string;
  priorities: string[];
};

export type Workspace = {
  slug: string;
  name: string;
  canWrite: boolean;
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

export const post = <T,>(path: string, body: unknown) =>
  call<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Gemba": "1" },
    body: JSON.stringify(body),
  });

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
