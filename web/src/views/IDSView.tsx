import { useEffect, useState } from "react";
import { apiClient } from "../api/client";

export function IDSView({ project }: { project: string }) {
  const [data, setData] = useState("Select a project");

  useEffect(() => {
    if (!project) {
      setData("Select a project");
      return;
    }
    apiClient
      .ids(project)
      .then((d) => setData(JSON.stringify(d, null, 2)))
      .catch((e: Error) => setData(e.message));
  }, [project]);

  return (
    <div>
      <h2>IDS export</h2>
      <pre className="pre">{data}</pre>
    </div>
  );
}
