import { useEffect, useState } from "react";
import { apiClient, type Patent } from "../api/client";

export function DetailView({ number, project }: { number: string; project: string }) {
  const [patent, setPatent] = useState<Patent | null>(null);
  const [ai, setAi] = useState("");
  const [err, setErr] = useState("");

  useEffect(() => {
    apiClient
      .patent(number)
      .then((r) => setPatent(r.patent))
      .catch((e: Error) => setErr(e.message));
  }, [number]);

  if (err) return <p>{err}</p>;
  if (!patent) return <p>Loading…</p>;

  return (
    <div>
      <h2>{patent.number as string}</h2>
      <p>{(patent.title as string) ?? ""}</p>
      <div className="toolbar">
        {project && (
          <>
            <button
              type="button"
              onClick={() =>
                apiClient.reviewState(number, project, "under_review")
              }
            >
              Under review
            </button>
            <button
              type="button"
              className="secondary"
              onClick={() => apiClient.crawl({ root: number, depth: 0, profile: "family", project })}
            >
              Crawl family
            </button>
          </>
        )}
        <button
          type="button"
          className="secondary"
          onClick={async () => {
            const r = await apiClient.aiAnalyze(number, "novelty");
            setAi(r.summary);
          }}
        >
          AI novelty
        </button>
      </div>
      <pre className="pre">{JSON.stringify(patent, null, 2)}</pre>
      {ai && (
        <>
          <h3>AI summary</h3>
          <pre className="pre">{ai}</pre>
        </>
      )}
    </div>
  );
}
