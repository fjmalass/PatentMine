import { useEffect, useState } from "react";
import { apiClient } from "../api/client";

export function SettingsView() {
  const [token, setToken] = useState(localStorage.getItem("patentmine_api_token") ?? "");
  const [ai, setAi] = useState("");

  useEffect(() => {
    apiClient
      .aiConfig()
      .then((r) => setAi(JSON.stringify(r, null, 2)))
      .catch(() => setAi("unavailable"));
  }, []);

  return (
    <div>
      <h2>Settings</h2>
      <p className="status">API token (stored in browser localStorage)</p>
      <input
        value={token}
        onChange={(e) => setToken(e.target.value)}
        style={{ width: "100%", maxWidth: 480 }}
        placeholder="Bearer token for remote API"
      />
      <div className="toolbar">
        <button
          type="button"
          onClick={() => {
            localStorage.setItem("patentmine_api_token", token);
            alert("Saved");
          }}
        >
          Save token
        </button>
      </div>
      <h3>Server AI</h3>
      <pre className="pre">{ai}</pre>
    </div>
  );
}
