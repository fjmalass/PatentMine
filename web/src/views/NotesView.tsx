import { useEffect, useState } from "react";

export function NotesView({ project }: { project: string }) {
  const [md, setMd] = useState("");

  useEffect(() => {
    if (!project) {
      setMd("Select a project");
      return;
    }
    fetch(
      `${import.meta.env.VITE_API_BASE ?? ""}/projects/${project}/notes/export`,
      { headers: auth() },
    )
      .then((r) => r.text())
      .then(setMd)
      .catch((e: Error) => setMd(e.message));
  }, [project]);

  return (
    <div>
      <h2>Project notes</h2>
      <pre className="pre">{md}</pre>
    </div>
  );
}

function auth(): HeadersInit {
  const token = localStorage.getItem("patentmine_api_token");
  const h: Record<string, string> = {};
  if (token) h.Authorization = `Bearer ${token}`;
  return h;
}
