package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pipescope/backend/internal/correlator"
	"github.com/pipescope/backend/internal/graph"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	sampleDir := filepath.Join("..", "..", "..", "sample-pipeline")
	overridesPath := filepath.Join(t.TempDir(), "data", "overrides.json")
	srv := NewServer(sampleDir, overridesPath)
	srv.SimStepDuration = 10 * time.Millisecond
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return srv, ts
}

func TestHandleHealthAndSamplePipeline(t *testing.T) {
	_, ts := newTestServer(t)

	// 1. Health endpoint
	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/health, got %d", resp.StatusCode)
	}

	// 2. Sample pipeline endpoint
	sampleResp, err := http.Get(ts.URL + "/api/pipeline/sample")
	if err != nil {
		t.Fatalf("GET /api/pipeline/sample failed: %v", err)
	}
	defer sampleResp.Body.Close()
	if sampleResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/pipeline/sample, got %d", sampleResp.StatusCode)
	}

	var g graph.PipelineGraph
	if err := json.NewDecoder(sampleResp.Body).Decode(&g); err != nil {
		t.Fatalf("failed to decode PipelineGraph response: %v", err)
	}
	if len(g.Nodes) == 0 || len(g.Edges) == 0 || len(g.Groups) == 0 {
		t.Fatalf("expected populated PipelineGraph from /api/pipeline/sample, got %+v", g)
	}
	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG from /api/pipeline/sample, got: %v", err)
	}
}

func TestHandleParseFilesAndManualOverride(t *testing.T) {
	_, ts := newTestServer(t)

	filesPayload := []correlator.FileInput{
		{
			Path: ".github/workflows/ci.yml",
			Content: `
name: Quick CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: docker build -t ghcr.io/pipescope/demo:v1 .
`,
		},
		{
			Path: "Dockerfile",
			Content: `
FROM alpine:3.20
EXPOSE 8080
`,
		},
	}

	rawBody, _ := json.Marshal(filesPayload)
	resp, err := http.Post(ts.URL+"/api/pipeline/parse-files", "application/json", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatalf("POST /api/pipeline/parse-files failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/pipeline/parse-files, got %d", resp.StatusCode)
	}

	var parsedGraph graph.PipelineGraph
	if err := json.NewDecoder(resp.Body).Decode(&parsedGraph); err != nil {
		t.Fatalf("failed to decode parsed graph: %v", err)
	}
	if len(parsedGraph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (gh-source-repo, gha-job-build, docker-image), got %d", len(parsedGraph.Nodes))
	}

	// Test POST /api/pipeline/manual-override
	overrideReq := ManualOverrideState{
		CustomNodes: []graph.Node{
			{
				ID:        "custom-s3-bucket",
				Label:     "AWS S3 Artifact Store",
				Stage:     graph.StageCloud,
				Tool:      graph.ToolAWS,
				ZoomLevel: 2,
			},
		},
		CustomEdges: []graph.Edge{
			{
				Source:       "docker-image",
				Target:       "custom-s3-bucket",
				RelationType: graph.RelationManualLink,
				Label:        "archives layer cache",
			},
		},
		NodePositions: map[string]graph.Position{
			"gha-job-build": {X: 300, Y: 150},
		},
	}
	overrideBytes, _ := json.Marshal(overrideReq)
	ovResp, err := http.Post(ts.URL+"/api/pipeline/manual-override", "application/json", bytes.NewReader(overrideBytes))
	if err != nil {
		t.Fatalf("POST /api/pipeline/manual-override failed: %v", err)
	}
	defer ovResp.Body.Close()
	if ovResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/pipeline/manual-override, got %d", ovResp.StatusCode)
	}

	var updatedGraph graph.PipelineGraph
	if err := json.NewDecoder(ovResp.Body).Decode(&updatedGraph); err != nil {
		t.Fatalf("failed to decode override response: %v", err)
	}
	if len(updatedGraph.Nodes) != 4 {
		t.Fatalf("expected 4 nodes after adding custom node, got %d", len(updatedGraph.Nodes))
	}
}

func TestHandleSimulateRunAndWebSocketTelemetry(t *testing.T) {
	_, ts := newTestServer(t)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/telemetry"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to /ws/telemetry: %v", err)
	}
	defer wsConn.Close()

	// Trigger simulation run
	resp, err := http.Post(ts.URL+"/api/pipeline/simulate-run", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/pipeline/simulate-run failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted from /api/pipeline/simulate-run, got %d", resp.StatusCode)
	}

	var sawRunningUpdate, sawCompletion bool
	deadline := time.Now().Add(3 * time.Second)
	_ = wsConn.SetReadDeadline(deadline)

	for time.Now().Before(deadline) {
		var evt TelemetryEvent
		if err := wsConn.ReadJSON(&evt); err != nil {
			break
		}
		if evt.Type == "telemetry_update" {
			sawRunningUpdate = true
		}
		if evt.Type == "simulation_completed" {
			sawCompletion = true
			break
		}
	}

	if !sawRunningUpdate {
		t.Errorf("expected to receive at least one 'telemetry_update' WebSocket event")
	}
	if !sawCompletion {
		t.Errorf("expected to receive 'simulation_completed' WebSocket event")
	}
}
