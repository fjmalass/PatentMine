import { useCallback, useEffect, useState } from "react";
import { Link, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { apiClient, connectEvents, type Project } from "./api/client";
import { CatalogView } from "./views/CatalogView";
import { DetailView } from "./views/DetailView";
import { FamilyView } from "./views/FamilyView";
import { RelationsView } from "./views/RelationsView";
import { IDSView } from "./views/IDSView";
import { NotesView } from "./views/NotesView";
import { MetricsView } from "./views/MetricsView";
import { SettingsView } from "./views/SettingsView";

export function App() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [project, setProject] = useState("");
  const [status, setStatus] = useState("Connecting…");
  const navigate = useNavigate();

  const refreshProjects = useCallback(async () => {
    const res = await apiClient.projects();
    setProjects(res.projects ?? []);
  }, []);

  useEffect(() => {
    apiClient
      .health()
      .then(() => {
        setStatus("Daemon OK");
        return refreshProjects();
      })
      .catch((e: Error) => setStatus(e.message));
  }, [refreshProjects]);

  useEffect(() => {
    apiClient.session.get().then((s) => {
      if (s.active_project) setProject(s.active_project);
    });
  }, []);

  useEffect(() => {
    const stop = connectEvents((ev) => {
      const e = ev as { method?: string };
      if (e.method === "db.changed" || e.method === "session.changed") {
        setStatus(`Event: ${e.method}`);
      }
      if (e.method === "crawl.progress") {
        setStatus(`Crawl in progress`);
      }
    });
    return stop;
  }, []);

  const onProject = (id: string) => {
    setProject(id);
    apiClient.session.patch({ active_project: id, view: "catalog" });
  };

  const openPatent = (n: string) => {
    apiClient.session.patch({
      active_project: project,
      focused_patent: n,
      view: "detail",
    });
    navigate(`/patent/${encodeURIComponent(n)}`);
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <h1>PatentMine</h1>
        <p className="status">{status}</p>
        <label>
          Project
          <select
            value={project}
            onChange={(e) => onProject(e.target.value)}
            style={{ display: "block", width: "100%", marginTop: 4 }}
          >
            <option value="">— none —</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <nav style={{ marginTop: "1rem" }}>
          <Link to="/">Catalog</Link>
          <br />
          <Link to="/orphans">Orphans</Link>
          <br />
          <Link to="/ids">IDS</Link>
          <br />
          <Link to="/notes">Notes</Link>
          <br />
          <Link to="/metrics">Metrics</Link>
          <br />
          <Link to="/settings">Settings</Link>
        </nav>
      </aside>
      <main className="main">
        <Routes>
          <Route path="/" element={<CatalogView project={project} onOpen={openPatent} />} />
          <Route
            path="/orphans"
            element={<CatalogView project="" orphans onOpen={openPatent} />}
          />
          <Route path="/patent/:number" element={<PatentDetail project={project} />} />
          <Route path="/patent/:number/citations" element={<PatentRelations project={project} kind="cites" />} />
          <Route path="/patent/:number/citedby" element={<PatentRelations project={project} kind="cited_by" />} />
          <Route path="/patent/:number/family" element={<PatentFamily project={project} />} />
          <Route path="/patent/:number/fulltext" element={<PatentFulltext />} />
          <Route path="/patent/:number/diffs" element={<PatentDiffs />} />
          <Route path="/ids" element={<IDSView project={project} />} />
          <Route path="/notes" element={<NotesView project={project} />} />
          <Route path="/metrics" element={<MetricsView />} />
          <Route path="/settings" element={<SettingsView />} />
        </Routes>
      </main>
    </div>
  );
}

function patentNumber(): string {
  const { number = "" } = useParams();
  return decodeURIComponent(number);
}

function PatentDetail({ project }: { project: string }) {
  return <DetailView number={patentNumber()} project={project} />;
}
function PatentRelations({ project, kind }: { project: string; kind: string }) {
  return <RelationsView number={patentNumber()} project={project} kind={kind} />;
}
function PatentFamily({ project }: { project: string }) {
  return <FamilyView number={patentNumber()} project={project} />;
}
function PatentFulltext() {
  return <FulltextPane number={patentNumber()} />;
}
function PatentDiffs() {
  return <DiffsPane number={patentNumber()} />;
}

function FulltextPane({ number }: { number: string }) {
  const [text, setText] = useState("Loading…");
  useEffect(() => {
    apiClient.grantBody(number).then(setText).catch((e: Error) => setText(e.message));
  }, [number]);
  return (
    <div>
      <h2>Full text — {number}</h2>
      <pre className="pre">{text}</pre>
    </div>
  );
}

function DiffsPane({ number }: { number: string }) {
  const [data, setData] = useState("Loading…");
  useEffect(() => {
    apiClient
      .diffs(number)
      .then((d) => setData(JSON.stringify(d, null, 2)))
      .catch((e: Error) => setData(e.message));
  }, [number]);
  return (
    <div>
      <h2>Source diffs — {number}</h2>
      <pre className="pre">{data}</pre>
    </div>
  );
}
