import { useEffect, useState } from "react";
import { apiClient } from "../api/client";

export function MetricsView() {
  const [data, setData] = useState("");

  useEffect(() => {
    apiClient.metrics().then((d) => setData(JSON.stringify(d, null, 2)));
  }, []);

  return (
    <div>
      <h2>Daemon metrics</h2>
      <pre className="pre">{data}</pre>
    </div>
  );
}
