const API_BASE = import.meta.env.VITE_API_BASE ?? "";

function authHeaders(): HeadersInit {
  const token = localStorage.getItem("patentmine_api_token");
  const h: Record<string, string> = { "Content-Type": "application/json" };
  if (token) h.Authorization = `Bearer ${token}`;
  return h;
}

export async function api<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { ...authHeaders(), ...init.headers },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error ?? res.statusText);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") ?? "";
  if (ct.includes("text/markdown") || ct.includes("text/plain")) {
    return (await res.text()) as T;
  }
  return res.json() as Promise<T>;
}

export type Project = { id: string; name: string };
export type PatentRow = {
  number: string;
  title?: string;
  review_state?: string;
};
export type Patent = Record<string, unknown> & { number: string };

export function connectEvents(onEvent: (ev: unknown) => void): () => void {
  const token = localStorage.getItem("patentmine_api_token");
  let path = `${API_BASE}/events`;
  if (token) path += `?token=${encodeURIComponent(token)}`;
  const es = new EventSource(path);
  es.addEventListener("daemon", (e) => {
    try {
      onEvent(JSON.parse(e.data));
    } catch {
      /* ignore */
    }
  });
  return () => es.close();
}

export const apiClient = {
  health: () => api<{ pong: boolean }>("/healthz"),
  projects: () => api<{ projects: Project[] }>("/projects"),
  patents: (q: Record<string, string>) => {
    const p = new URLSearchParams(q);
    return api<{ patents: PatentRow[]; total: number }>(`/patents?${p}`);
  },
  patent: (n: string) => api<{ patent: Patent }>(`/patents/${encodeURIComponent(n)}`),
  relations: (n: string, kind: string, q: Record<string, string>) => {
    const p = new URLSearchParams({ kind, ...q });
    return api<{ patents: PatentRow[]; total: number }>(
      `/patents/${encodeURIComponent(n)}/relations?${p}`,
    );
  },
  family: (n: string, depth: number, project?: string) => {
    const p = new URLSearchParams({ depth: String(depth) });
    if (project) p.set("project", project);
    return api<unknown>(`/patents/${encodeURIComponent(n)}/family?${p}`);
  },
  reviewState: (n: string, project: string, state: string) =>
    api(`/patents/${encodeURIComponent(n)}/review_state`, {
      method: "PUT",
      body: JSON.stringify({ project, state }),
    }),
  crawl: (body: unknown) =>
    api("/crawl", { method: "POST", body: JSON.stringify(body) }),
  session: {
    get: () => api<Record<string, string>>("/session"),
    patch: (body: Record<string, string>) =>
      api("/session", { method: "PATCH", body: JSON.stringify(body) }),
  },
  commands: () => api<unknown[]>("/commands"),
  metrics: () => api<unknown>("/metricsz"),
  aiConfig: () => api<{ configured: boolean; provider: string }>("/ai/config"),
  aiAnalyze: (n: string, action: string) =>
    api<{ summary: string }>(`/patents/${encodeURIComponent(n)}/ai/analyze`, {
      method: "POST",
      body: JSON.stringify({ action }),
    }),
  note: {
    get: (project: string, n: string) =>
      api(`/projects/${project}/patents/${encodeURIComponent(n)}/note`),
    save: (project: string, n: string, body: unknown) =>
      api(`/projects/${project}/patents/${encodeURIComponent(n)}/note`, {
        method: "PUT",
        body: JSON.stringify(body),
      }),
  },
  ids: (project: string) => api(`/projects/${project}/ids`),
  diffs: (n: string) =>
    api(`/patents/${encodeURIComponent(n)}/diffs`),
  grantBody: (n: string) =>
    api<string>(`/patents/${encodeURIComponent(n)}/uspto/grant_body`),
  validateFilter: (expression: string, projectActive: boolean) =>
    api<{ canonical: string }>("/filters/validate", {
      method: "POST",
      body: JSON.stringify({ expression, project_active: projectActive }),
    }),
};
