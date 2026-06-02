import { useEffect, useState } from "react";
import { apiClient } from "../api/client";

export function FamilyView({ number, project }: { number: string; project: string }) {
  const [depth, setDepth] = useState(2);
  const [data, setData] = useState("");

  useEffect(() => {
    apiClient
      .family(number, depth, project || undefined)
      .then((d) => setData(JSON.stringify(d, null, 2)))
      .catch((e: Error) => setData(e.message));
  }, [number, project, depth]);

  return (
    <div>
      <h2>Family graph — {number}</h2>
      <div className="toolbar">
        <label>
          Depth{" "}
          <input
            type="number"
            min={0}
            max={6}
            value={depth}
            onChange={(e) => setDepth(Number(e.target.value))}
          />
        </label>
      </div>
      <pre className="pre">{data}</pre>
    </div>
  );
}
