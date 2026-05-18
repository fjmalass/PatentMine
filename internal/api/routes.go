package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// routes registers every HTTP endpoint. Each maps to a command in the registry
// (named in the trailing comment), so the web API and the TUI act on the same
// catalogue of operations.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /commands", s.handleCommands)
	s.mux.HandleFunc("GET /patents", s.handlePatentList)                   // command.PatentList
	s.mux.HandleFunc("GET /patents/{number}", s.handlePatentGet)           // command.PatentGet
	s.mux.HandleFunc("GET /patents/{number}/relations", s.handleRelations) // command.PatentRelations
	s.mux.HandleFunc("PUT /patents/{number}/state", s.handleState)         // command.Mark*
	s.mux.HandleFunc("GET /projects", s.handleProjectList)                 // command.ProjectList
	s.mux.HandleFunc("POST /projects", s.handleProjectCreate)              // command.ProjectCreate
	s.mux.HandleFunc("POST /projects/{id}/patents", s.handleAddMember)     // command.AddToProject
	s.mux.HandleFunc("GET /projects/{id}/ids", s.handleIDS)                // command.ExportIDS
	s.mux.HandleFunc("POST /ingest", s.handleIngest)                       // command.IngestFamily
}

// handleRelations lists a patent's family-graph edges of one kind (the "kind"
// query parameter; defaults to citations).
func (s *Server) handleRelations(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = string(domain.RelationCites)
	}
	var res proto.RelationsResult
	s.call(w, r, proto.MethodRelations,
		proto.RelationsParams{Number: number, Kind: kind}, &res)
}

// handleIDS builds the Information Disclosure Statement for a project.
func (s *Server) handleIDS(w http.ResponseWriter, r *http.Request) {
	var res proto.IDSResult
	s.call(w, r, proto.MethodIDSExport,
		proto.IDSExportParams{Project: domain.ProjectID(r.PathValue("id"))}, &res)
}

// handleHealth pings the daemon.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	var res proto.PingResult
	s.call(w, r, proto.MethodPing, nil, &res)
}

// handleCommands returns the command registry — the shared source of truth for
// every frontend's actions.
func (s *Server) handleCommands(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.registry.All())
}

// handlePatentList lists patents, paginated via query parameters.
func (s *Server) handlePatentList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	params := proto.PatentListParams{
		Project: q.Get("project"),
		State:   q.Get("state"),
		Search:  q.Get("search"),
		Limit:   limit,
		Offset:  offset,
	}
	var res proto.PatentListResult
	s.call(w, r, proto.MethodPatentList, params, &res)
}

// handlePatentGet returns one patent.
func (s *Server) handlePatentGet(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var res proto.PatentResult
	s.call(w, r, proto.MethodPatentGet, proto.PatentGetParams{Number: number}, &res)
}

// handleState changes a patent's membership state in a project.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var body struct {
		Project string `json:"project"`
		State   string `json:"state"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.MembershipStateParams{
		Project: domain.ProjectID(body.Project),
		Patent:  number,
		State:   body.State,
	}
	var res proto.Empty
	s.call(w, r, proto.MethodMembershipState, params, &res)
}

// handleProjectList lists projects.
func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	var res proto.ProjectListResult
	s.call(w, r, proto.MethodProjectList, nil, &res)
}

// handleProjectCreate creates a project.
func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.ProjectResult
	s.call(w, r, proto.MethodProjectCreate, proto.ProjectCreateParams{Name: body.Name}, &res)
}

// handleAddMember adds a patent to a project.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Patent domain.PatentNumber `json:"patent"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.MembershipParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patent:  body.Patent,
	}
	var res proto.Empty
	s.call(w, r, proto.MethodMembershipAdd, params, &res)
}

// handleIngest starts a family-graph crawl.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Root  domain.PatentNumber `json:"root"`
		Depth int                 `json:"depth"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.IngestStartResult
	s.call(w, r, proto.MethodIngestFamily,
		proto.IngestFamilyParams{Root: body.Root, Depth: body.Depth}, &res)
}

// decodeBody decodes a JSON request body, writing a 400 on failure. It reports
// whether decoding succeeded.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
