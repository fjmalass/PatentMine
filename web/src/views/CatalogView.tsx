import { useCallback, useEffect, useState } from "react";
import { apiClient, type PatentRow } from "../api/client";

type Props = {
  project: string;
  orphans?: boolean;
  onOpen: (n: string) => void;
};

export function CatalogView({ project, orphans, onOpen }: Props) {
  const [rows, setRows] = useState<PatentRow[]>([]);
  const [total, setTotal] = useState(0);
  const [filter, setFilter] = useState("fetch_state:cached");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [err, setErr] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      if (orphans) {
        const o = await fetch(
          `${import.meta.env.VITE_API_BASE ?? ""}/patents/orphans?limit=100`,
          { headers: auth() },
        ).then((r) => r.json());
        setRows(o.patents ?? []);
        setTotal(o.total ?? 0);
        return;
      }
      const q: Record<string, string> = {
        limit: "100",
        offset: "0",
        filter,
      };
      if (project) q.project = project;
      if (search) q.search = search;
      const res = await apiClient.patents(q);
      setRows(res.patents ?? []);
      setTotal(res.total ?? 0);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [filter, project, search, orphans]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div>
      <h2>{orphans ? "Orphans" : "Patent catalog"}</h2>
      <div className="toolbar">
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="filter expression"
          style={{ flex: 1, minWidth: 200 }}
        />
        <button
          type="button"
          className="secondary"
          onClick={async () => {
            try {
              const r = await apiClient.validateFilter(filter, !!project);
              setFilter(r.canonical);
            } catch (e) {
              setErr((e as Error).message);
            }
          }}
        >
          Validate
        </button>
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="search"
        />
        <button type="button" onClick={load}>
          Refresh
        </button>
      </div>
      {err && <p className="status">{err}</p>}
      <p className="status">
        {total} patents · click row to open detail (Enter)
      </p>
      <table>
        <thead>
          <tr>
            <th>Number</th>
            <th>Title</th>
            <th>State</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr
              key={r.number}
              className={selected === r.number ? "selected" : ""}
              onClick={() => setSelected(r.number)}
              onDoubleClick={() => onOpen(r.number)}
            >
              <td>{r.number}</td>
              <td>{r.title ?? "—"}</td>
              <td>{r.review_state ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {selected && (
        <div className="toolbar">
          <button type="button" onClick={() => onOpen(selected)}>
            Open detail
          </button>
          {project && (
            <>
              <button
                type="button"
                className="secondary"
                onClick={() =>
                  apiClient.reviewState(selected, project, "active").then(load)
                }
              >
                Mark active
              </button>
              <button
                type="button"
                className="danger"
                onClick={() =>
                  fetch(
                    `${import.meta.env.VITE_API_BASE ?? ""}/patents/${encodeURIComponent(selected)}`,
                    { method: "DELETE", headers: auth() },
                  ).then(load)
                }
              >
                Delete
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}

function auth(): HeadersInit {
  const token = localStorage.getItem("patentmine_api_token");
  const h: Record<string, string> = {};
  if (token) h.Authorization = `Bearer ${token}`;
  return h;
}
