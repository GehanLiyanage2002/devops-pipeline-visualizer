package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/pipescope/backend/internal/correlator"
	"github.com/pipescope/backend/internal/graph"
)

// ManualOverrideState represents the persisted manual overrides stored in data/overrides.json.
type ManualOverrideState struct {
	CustomNodes    []graph.Node              `json:"custom_nodes"`
	CustomEdges    []graph.Edge              `json:"custom_edges"`
	NodePositions  map[string]graph.Position `json:"node_positions"`
	DeleteNodeIDs  []string                  `json:"delete_node_ids,omitempty"`
	ClearPositions bool                      `json:"clear_positions,omitempty"`
	Reset          bool                      `json:"reset,omitempty"`
}

// Server holds the API state, active PipelineGraph, override persistence, and WebSocket hub.
type Server struct {
	mu              sync.RWMutex
	sampleDir       string
	overridesPath   string
	activeGraph     *graph.PipelineGraph
	overrides       ManualOverrideState
	hub             *TelemetryHub
	SimStepDuration time.Duration
	simRunning      bool
	httpClient      *http.Client
}

// NewServer creates a new PipeScope API server instance.
func NewServer(sampleDir, overridesPath string) *Server {
	s := &Server{
		sampleDir:       sampleDir,
		overridesPath:   overridesPath,
		overrides:       ManualOverrideState{NodePositions: make(map[string]graph.Position)},
		hub:             NewTelemetryHub(),
		SimStepDuration: 2 * time.Second, // 6 stages * 2s = 12 seconds total simulation run
		httpClient:      &http.Client{Timeout: 20 * time.Second},
	}
	s.loadOverridesFromDisk()
	return s
}

// Router builds and returns the configured chi.Router with CORS and all PipeScope endpoints.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/api/health", s.HandleHealth)
	r.Get("/api/pipeline/sample", s.HandleGetSamplePipeline)
	r.Post("/api/pipeline/parse-files", s.HandleParseFiles)
	r.Post("/api/pipeline/scan-github", s.HandleScanGitHub)
	r.Post("/api/pipeline/manual-override", s.HandleManualOverride)
	r.Post("/api/pipeline/simulate-run", s.HandleSimulateRun)
	r.Get("/ws/telemetry", s.hub.HandleWebSocket)

	return r
}

// HandleHealth handles GET /api/health.
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"service":   "pipescope-backend",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleGetSamplePipeline handles GET /api/pipeline/sample.
func (s *Server) HandleGetSamplePipeline(w http.ResponseWriter, r *http.Request) {
	g, err := correlator.ScanAndStitchDirectory(s.sampleDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to scan sample pipeline: %v", err))
		return
	}

	s.mu.Lock()
	correlator.ApplyManualOverrides(g, s.overrides.CustomNodes, s.overrides.CustomEdges, s.overrides.NodePositions)
	s.activeGraph = cloneGraph(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, g)
}

type parseFilesRequestObject struct {
	Files         []correlator.FileInput `json:"files"`
	DirectoryPath string                 `json:"directory_path"`
}

// HandleParseFiles handles POST /api/pipeline/parse-files.
// Supports either a direct JSON array `[{ "path": "...", "content": "..." }]`
// or an object `{ "files": [...], "directory_path": "..." }`.
func (s *Server) HandleParseFiles(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		writeError(w, http.StatusBadRequest, "empty request body")
		return
	}

	var (
		files   []correlator.FileInput
		dirPath string
	)

	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &files); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid files array JSON: %v", err))
			return
		}
	} else {
		var reqObj parseFilesRequestObject
		if err := json.Unmarshal(trimmed, &reqObj); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request JSON: %v", err))
			return
		}
		files = reqObj.Files
		dirPath = strings.TrimSpace(reqObj.DirectoryPath)
	}

	var g *graph.PipelineGraph
	if dirPath != "" {
		g, err = correlator.ScanAndStitchDirectory(dirPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to scan directory %q: %v", dirPath, err))
			return
		}
	} else {
		g, err = correlator.ParseAndStitchFiles(files)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse uploaded files: %v", err))
			return
		}
	}

	s.mu.Lock()
	correlator.ApplyManualOverrides(g, s.overrides.CustomNodes, s.overrides.CustomEdges, s.overrides.NodePositions)
	s.activeGraph = cloneGraph(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, g)
}

type scanGitHubRequest struct {
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch"`
	GitHubToken string `json:"github_token"`
}

type githubTreeResponse struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

// HandleScanGitHub handles POST /api/pipeline/scan-github.
func (s *Server) HandleScanGitHub(w http.ResponseWriter, r *http.Request) {
	var req scanGitHubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON payload: %v", err))
		return
	}

	owner, repo, err := parseGitHubOwnerRepo(req.RepoURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}

	treeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, url.PathEscape(branch))
	treeReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, treeURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	treeReq.Header.Set("Accept", "application/vnd.github+json")
	treeReq.Header.Set("User-Agent", "PipeScope-Analyzer/1.0")
	if strings.TrimSpace(req.GitHubToken) != "" {
		treeReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(req.GitHubToken))
	}

	resp, err := s.httpClient.Do(treeReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to contact GitHub API: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("GitHub API returned %d: %s", resp.StatusCode, string(msg)))
		return
	}

	var treeData githubTreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&treeData); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to decode GitHub tree response: %v", err))
		return
	}

	var files []correlator.FileInput
	for _, entry := range treeData.Tree {
		if entry.Type != "blob" || !isCandidateDevOpsFile(entry.Path) {
			continue
		}
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, branch, entry.Path)
		rawReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, rawURL, nil)
		if err != nil {
			continue
		}
		if strings.TrimSpace(req.GitHubToken) != "" {
			rawReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(req.GitHubToken))
		}
		fileResp, err := s.httpClient.Do(rawReq)
		if err != nil {
			continue
		}
		if fileResp.StatusCode == http.StatusOK {
			contentBytes, _ := io.ReadAll(fileResp.Body)
			files = append(files, correlator.FileInput{
				Path:    entry.Path,
				Content: string(contentBytes),
			})
		}
		_ = fileResp.Body.Close()
	}

	g, err := correlator.ParseAndStitchFiles(files)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("failed to parse repository configs: %v", err))
		return
	}
	g.Name = fmt.Sprintf("%s/%s (%s)", owner, repo, branch)

	s.mu.Lock()
	correlator.ApplyManualOverrides(g, s.overrides.CustomNodes, s.overrides.CustomEdges, s.overrides.NodePositions)
	s.activeGraph = cloneGraph(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, g)
}

// HandleManualOverride handles POST /api/pipeline/manual-override.
func (s *Server) HandleManualOverride(w http.ResponseWriter, r *http.Request) {
	var payload ManualOverrideState
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid override JSON: %v", err))
		return
	}

	s.mu.Lock()
	if payload.Reset {
		s.overrides = ManualOverrideState{
			NodePositions: make(map[string]graph.Position),
		}
	} else {
		if payload.ClearPositions || s.overrides.NodePositions == nil {
			s.overrides.NodePositions = make(map[string]graph.Position)
			if s.activeGraph != nil {
				for i := range s.activeGraph.Nodes {
					s.activeGraph.Nodes[i].Position = graph.Position{X: 0, Y: 0}
				}
			}
		}
		for nodeID, pos := range payload.NodePositions {
			s.overrides.NodePositions[nodeID] = pos
		}

		// Handle deletion of manual nodes
		if len(payload.DeleteNodeIDs) > 0 {
			deleteSet := make(map[string]bool, len(payload.DeleteNodeIDs))
			for _, id := range payload.DeleteNodeIDs {
				deleteSet[id] = true
				delete(s.overrides.NodePositions, id)
			}
			filteredCustomNodes := make([]graph.Node, 0, len(s.overrides.CustomNodes))
			for _, cn := range s.overrides.CustomNodes {
				if !deleteSet[cn.ID] {
					filteredCustomNodes = append(filteredCustomNodes, cn)
				}
			}
			s.overrides.CustomNodes = filteredCustomNodes

			filteredCustomEdges := make([]graph.Edge, 0, len(s.overrides.CustomEdges))
			for _, ce := range s.overrides.CustomEdges {
				if !deleteSet[ce.Source] && !deleteSet[ce.Target] {
					filteredCustomEdges = append(filteredCustomEdges, ce)
				}
			}
			s.overrides.CustomEdges = filteredCustomEdges

			if s.activeGraph != nil {
				remainingNodes := make([]graph.Node, 0, len(s.activeGraph.Nodes))
				for _, n := range s.activeGraph.Nodes {
					if !deleteSet[n.ID] {
						remainingNodes = append(remainingNodes, n)
					}
				}
				s.activeGraph.Nodes = remainingNodes

				remainingEdges := make([]graph.Edge, 0, len(s.activeGraph.Edges))
				for _, e := range s.activeGraph.Edges {
					if !deleteSet[e.Source] && !deleteSet[e.Target] {
						remainingEdges = append(remainingEdges, e)
					}
				}
				s.activeGraph.Edges = remainingEdges
			}
		}

		// Merge custom nodes by ID
		existingNodeIdx := make(map[string]int, len(s.overrides.CustomNodes))
		for i, n := range s.overrides.CustomNodes {
			existingNodeIdx[n.ID] = i
		}
		for _, cn := range payload.CustomNodes {
			cn.IsManual = true
			if idx, ok := existingNodeIdx[cn.ID]; ok {
				s.overrides.CustomNodes[idx] = cn
			} else {
				existingNodeIdx[cn.ID] = len(s.overrides.CustomNodes)
				s.overrides.CustomNodes = append(s.overrides.CustomNodes, cn)
			}
		}

		// Merge custom edges by ID
		existingEdgeIdx := make(map[string]int, len(s.overrides.CustomEdges))
		for i, e := range s.overrides.CustomEdges {
			existingEdgeIdx[e.ID] = i
		}
		for _, ce := range payload.CustomEdges {
			ce.IsManual = true
			if ce.RelationType == "" {
				ce.RelationType = graph.RelationManualLink
			}
			if ce.ID == "" {
				ce.ID = fmt.Sprintf("%s->%s:%s", ce.Source, ce.Target, ce.RelationType)
			}
			if idx, ok := existingEdgeIdx[ce.ID]; ok {
				s.overrides.CustomEdges[idx] = ce
			} else {
				existingEdgeIdx[ce.ID] = len(s.overrides.CustomEdges)
				s.overrides.CustomEdges = append(s.overrides.CustomEdges, ce)
			}
		}
	}

	_ = s.saveOverridesToDisk()

	if s.activeGraph == nil {
		if sampleGraph, err := correlator.ScanAndStitchDirectory(s.sampleDir); err == nil {
			s.activeGraph = sampleGraph
		} else {
			s.activeGraph = graph.NewPipelineGraph("pipescope-unified", "Unified DevOps Pipeline")
		}
	}

	correlator.ApplyManualOverrides(
		s.activeGraph,
		s.overrides.CustomNodes,
		s.overrides.CustomEdges,
		s.overrides.NodePositions,
	)
	updatedSnapshot := cloneGraph(s.activeGraph)
	s.mu.Unlock()

	s.hub.Broadcast(TelemetryEvent{
		Type:          "graph_updated",
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Message:       "Manual architecture overrides applied",
		NodeStatuses:  collectNodeStatuses(updatedSnapshot),
		AnimatedEdges: []string{},
		Graph:         updatedSnapshot,
	})

	writeJSON(w, http.StatusOK, updatedSnapshot)
}

// HandleSimulateRun handles POST /api/pipeline/simulate-run.
// Runs a realistic 6-stage CI/CD deployment simulation across the active graph over 12 seconds.
func (s *Server) HandleSimulateRun(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.activeGraph == nil {
		if sampleGraph, err := correlator.ScanAndStitchDirectory(s.sampleDir); err == nil {
			correlator.ApplyManualOverrides(sampleGraph, s.overrides.CustomNodes, s.overrides.CustomEdges, s.overrides.NodePositions)
			s.activeGraph = sampleGraph
		} else {
			s.mu.Unlock()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to initialize pipeline graph: %v", err))
			return
		}
	}
	if s.simRunning {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":  "simulation_already_running",
			"message": "A pipeline simulation is already in progress",
		})
		return
	}
	s.simRunning = true
	stepDuration := s.SimStepDuration
	if stepDuration <= 0 {
		stepDuration = 2 * time.Second
	}
	s.mu.Unlock()

	go s.runPipelineSimulation(stepDuration)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":           "simulation_started",
		"duration_seconds": int((stepDuration * 6).Seconds()),
	})
}

func (s *Server) runPipelineSimulation(stepDuration time.Duration) {
	defer func() {
		s.mu.Lock()
		s.simRunning = false
		s.mu.Unlock()
	}()

	phases := []struct {
		Name  string
		Tools map[graph.Tool]bool
	}{
		{
			Name:  "source",
			Tools: map[graph.Tool]bool{graph.ToolGitHub: true},
		},
		{
			Name:  "ci",
			Tools: map[graph.Tool]bool{graph.ToolGitHubActions: true},
		},
		{
			Name:  "artifact",
			Tools: map[graph.Tool]bool{graph.ToolDocker: true},
		},
		{
			Name:  "iac_and_config_mgmt",
			Tools: map[graph.Tool]bool{graph.ToolTerraform: true, graph.ToolAnsible: true},
		},
		{
			Name:  "gitops",
			Tools: map[graph.Tool]bool{graph.ToolArgoCD: true},
		},
		{
			Name:  "runtime_and_cloud",
			Tools: map[graph.Tool]bool{graph.ToolKubernetes: true, graph.ToolAWS: true, graph.ToolAzure: true},
		},
	}

	// Reset all nodes to idle at simulation start
	s.mu.Lock()
	for i := range s.activeGraph.Nodes {
		s.activeGraph.Nodes[i].Status = graph.StatusIdle
	}
	for i := range s.activeGraph.Edges {
		s.activeGraph.Edges[i].Animated = false
	}
	s.mu.Unlock()

	completedTools := make(map[graph.Tool]bool)

	for _, phase := range phases {
		s.mu.Lock()
		runningNodeIDs := make(map[string]bool)
		for i := range s.activeGraph.Nodes {
			n := &s.activeGraph.Nodes[i]
			switch {
			case phase.Tools[n.Tool]:
				n.Status = graph.StatusRunning
				runningNodeIDs[n.ID] = true
			case completedTools[n.Tool]:
				n.Status = graph.StatusSuccess
			default:
				n.Status = graph.StatusIdle
			}
		}

		var animatedEdges []string
		for i := range s.activeGraph.Edges {
			e := &s.activeGraph.Edges[i]
			e.Animated = runningNodeIDs[e.Source] || runningNodeIDs[e.Target]
			if e.Animated {
				animatedEdges = append(animatedEdges, e.ID)
			}
		}

		snapshot := cloneGraph(s.activeGraph)
		statuses := collectNodeStatuses(snapshot)
		s.mu.Unlock()

		s.hub.Broadcast(TelemetryEvent{
			Type:          "telemetry_update",
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Stage:         phase.Name,
			Message:       fmt.Sprintf("Running stage: %s", phase.Name),
			NodeStatuses:  statuses,
			AnimatedEdges: animatedEdges,
			Graph:         snapshot,
		})

		time.Sleep(stepDuration)

		for tool := range phase.Tools {
			completedTools[tool] = true
		}
	}

	// Mark all nodes as success and stop edge animations at completion
	s.mu.Lock()
	for i := range s.activeGraph.Nodes {
		s.activeGraph.Nodes[i].Status = graph.StatusSuccess
	}
	for i := range s.activeGraph.Edges {
		s.activeGraph.Edges[i].Animated = false
	}
	finalSnapshot := cloneGraph(s.activeGraph)
	finalStatuses := collectNodeStatuses(finalSnapshot)
	s.mu.Unlock()

	s.hub.Broadcast(TelemetryEvent{
		Type:          "simulation_completed",
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Stage:         "completed",
		Message:       "Pipeline execution completed successfully",
		NodeStatuses:  finalStatuses,
		AnimatedEdges: []string{},
		Graph:         finalSnapshot,
	})
}

func (s *Server) loadOverridesFromDisk() {
	if s.overridesPath == "" {
		return
	}
	raw, err := os.ReadFile(s.overridesPath)
	if err != nil {
		return
	}
	var loaded ManualOverrideState
	if err := json.Unmarshal(raw, &loaded); err == nil {
		if loaded.NodePositions == nil {
			loaded.NodePositions = make(map[string]graph.Position)
		}
		s.overrides = loaded
	}
}

func (s *Server) saveOverridesToDisk() error {
	if s.overridesPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.overridesPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.overrides, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.overridesPath, raw, 0o644)
}

func collectNodeStatuses(g *graph.PipelineGraph) map[string]graph.NodeStatus {
	statuses := make(map[string]graph.NodeStatus, len(g.Nodes))
	for _, n := range g.Nodes {
		statuses[n.ID] = n.Status
	}
	return statuses
}

func cloneGraph(src *graph.PipelineGraph) *graph.PipelineGraph {
	if src == nil {
		return nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst graph.PipelineGraph
	if err := json.Unmarshal(raw, &dst); err != nil {
		return src
	}
	return &dst
}

func parseGitHubOwnerRepo(repoURL string) (string, string, error) {
	cleaned := strings.TrimSpace(repoURL)
	cleaned = strings.TrimSuffix(cleaned, ".git")
	cleaned = strings.TrimSuffix(cleaned, "/")

	if strings.HasPrefix(cleaned, "http://") || strings.HasPrefix(cleaned, "https://") {
		u, err := url.Parse(cleaned)
		if err != nil {
			return "", "", fmt.Errorf("invalid repo_url: %w", err)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("repo_url must contain owner and repo (e.g., https://github.com/owner/repo)")
		}
		return parts[0], parts[1], nil
	}

	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf("invalid repo_url %q", repoURL)
}

func isCandidateDevOpsFile(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)

	if strings.Contains(lower, ".github/workflows/") && (strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")) {
		return true
	}
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") || strings.HasSuffix(base, ".dockerfile") {
		return true
	}
	if strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".tfstate") {
		return true
	}
	if strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") {
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}
