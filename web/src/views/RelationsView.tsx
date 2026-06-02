import { useEffect, useState } from "react";
import { apiClient, type PatentRow } from "../api/client";

export function RelationsView({
  number,
  project,
  kind,
}: {
  number: string;
  project: string;
  kind: string;
}) {
  const [rows, setRows] = useState<PatentRow[]>([]);
  const [err, setErr] = useState("");

  useEffect(() => {
    const q: Record<string, string> = { limit: "200" };
    if (project) q.project = project;
    apiClient
      .relations(number, kind, q)
      .then((r) => setRows(r.patents ?? []))
      .catch((e: Error) => setErr(e.message));
  }, [number, project, kind]);

  return (
    <div>
      <h2>
        {kind} — {number}
      </h2>
      {err && <p>{err}</p>}
      <table>
        <thead>
          <tr>
            <th>Number</th>
            <th>Title</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.number}>
              <td>{r.number}</td>
              <td>{r.title ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
