package parsers

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pipescope/backend/internal/graph"
)

func sampleFilePath(t *testing.T, relPath string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "sample-pipeline", relPath)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("sample fixture %s not found: %v", p, err)
	}
	return filepath.ToSlash(p)
}

func TestGitHubActionsParser_SampleWorkflow(t *testing.T) {
	var p Parser = NewGitHubActionsParser()

	path := sampleFilePath(t, ".github/workflows/deploy.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read sample workflow: %v", err)
	}

	g, err := p.Parse(".github/workflows/deploy.yml", content)
	if err != nil {
		t.Fatalf("GitHubActionsParser.Parse failed: %v", err)
	}

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG from GitHubActionsParser, got: %v", err)
	}

	// Expect 1 GitHub source node + 3 GitHub Actions job nodes (test, build, provision)
	if len(g.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(g.Nodes))
	}

	nodesByID := make(map[string]graph.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		nodesByID[n.ID] = n
	}

	// Verify Source Node
	srcNode, ok := nodesByID["gh-source-repo"]
	if !ok {
		t.Fatalf("expected source node 'gh-source-repo' to exist")
	}
	if srcNode.Stage != graph.StageSource || srcNode.Tool != graph.ToolGitHub {
		t.Errorf("unexpected source node stage/tool: %s / %s", srcNode.Stage, srcNode.Tool)
	}
	triggers, _ := srcNode.Metadata["triggers"].([]string)
	if !slices.Contains(triggers, "push") || !slices.Contains(triggers, "pull_request") {
		t.Errorf("expected push and pull_request triggers, got %v", triggers)
	}
	branches, _ := srcNode.Metadata["branches"].([]string)
	if !slices.Contains(branches, "main") {
		t.Errorf("expected 'main' branch trigger, got %v", branches)
	}

	// Verify Build Job Node
	buildNode, ok := nodesByID["gha-job-build"]
	if !ok {
		t.Fatalf("expected job node 'gha-job-build' to exist")
	}
	if buildNode.Stage != graph.StageCI || buildNode.Tool != graph.ToolGitHubActions {
		t.Errorf("unexpected build node stage/tool: %s / %s", buildNode.Stage, buildNode.Tool)
	}
	if buildNode.Metadata["runs_on"] != "ubuntu-latest" {
		t.Errorf("expected runs_on=ubuntu-latest, got %v", buildNode.Metadata["runs_on"])
	}
	if buildNode.Metadata["image_repo"] != "ghcr.io/pipescope/api" {
		t.Errorf("expected image_repo='ghcr.io/pipescope/api', got %v", buildNode.Metadata["image_repo"])
	}
	if buildNode.Metadata["cloud_provider"] != "aws" {
		t.Errorf("expected cloud_provider='aws', got %v", buildNode.Metadata["cloud_provider"])
	}

	// Verify Provision Job Node
	provNode, ok := nodesByID["gha-job-provision"]
	if !ok {
		t.Fatalf("expected job node 'gha-job-provision' to exist")
	}
	if provNode.Metadata["executes_terraform"] != true {
		t.Errorf("expected executes_terraform=true on provision job, got %v", provNode.Metadata["executes_terraform"])
	}
	if provNode.Metadata["executes_ansible"] != true {
		t.Errorf("expected executes_ansible=true on provision job, got %v", provNode.Metadata["executes_ansible"])
	}
	if provNode.Metadata["ansible_playbook"] != "ansible/playbook.yml" {
		t.Errorf("expected ansible_playbook='ansible/playbook.yml', got %v", provNode.Metadata["ansible_playbook"])
	}

	// Verify Edges: gh-source-repo -> gha-job-test -> gha-job-build -> gha-job-provision
	expectedEdges := map[string]bool{
		"gh-source-repo->gha-job-test:triggers":      false,
		"gha-job-test->gha-job-build:triggers":       false,
		"gha-job-build->gha-job-provision:triggers":  false,
	}
	for _, e := range g.Edges {
		if _, exists := expectedEdges[e.ID]; exists {
			expectedEdges[e.ID] = true
		}
	}
	for edgeID, found := range expectedEdges {
		if !found {
			t.Errorf("expected edge %q not found in %+v", edgeID, g.Edges)
		}
	}
}

func TestDockerfileParser_SampleDockerfile(t *testing.T) {
	var p Parser = NewDockerfileParser()

	path := sampleFilePath(t, "Dockerfile")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read sample Dockerfile: %v", err)
	}

	g, err := p.Parse("Dockerfile", content)
	if err != nil {
		t.Fatalf("DockerfileParser.Parse failed: %v", err)
	}

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG from DockerfileParser, got: %v", err)
	}

	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 docker artifact node, got %d", len(g.Nodes))
	}

	node := g.Nodes[0]
	if node.Stage != graph.StageArtifact || node.Tool != graph.ToolDocker {
		t.Errorf("expected stage=artifact, tool=docker, got %s / %s", node.Stage, node.Tool)
	}

	baseImages, _ := node.Metadata["base_images"].([]string)
	if !slices.Equal(baseImages, []string{"golang:1.23-alpine", "alpine:3.20"}) {
		t.Errorf("unexpected base_images: %v", baseImages)
	}

	buildStages, _ := node.Metadata["build_stages"].([]string)
	if !slices.Equal(buildStages, []string{"builder", "runtime"}) {
		t.Errorf("unexpected build_stages: %v", buildStages)
	}

	exposedPorts, _ := node.Metadata["exposed_ports"].([]string)
	if !slices.Equal(exposedPorts, []string{"8080", "9090/tcp"}) {
		t.Errorf("unexpected exposed_ports: %v", exposedPorts)
	}
}

func TestAnsibleParser_SamplePlaybook(t *testing.T) {
	var p Parser = NewAnsibleParser()

	path := sampleFilePath(t, "ansible/playbook.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read sample Ansible playbook: %v", err)
	}

	g, err := p.Parse("ansible/playbook.yml", content)
	if err != nil {
		t.Fatalf("AnsibleParser.Parse failed: %v", err)
	}

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG from AnsibleParser, got: %v", err)
	}

	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 ansible play node, got %d", len(g.Nodes))
	}

	node := g.Nodes[0]
	if node.Stage != graph.StageConfigMgmt || node.Tool != graph.ToolAnsible {
		t.Errorf("expected stage=config_mgmt, tool=ansible, got %s / %s", node.Stage, node.Tool)
	}
	if node.Metadata["target_hosts"] != "app_servers" {
		t.Errorf("expected target_hosts='app_servers', got %v", node.Metadata["target_hosts"])
	}

	roles, _ := node.Metadata["roles"].([]string)
	if !slices.Equal(roles, []string{"common", "docker"}) {
		t.Errorf("expected roles [common, docker], got %v", roles)
	}

	tasks, _ := node.Metadata["tasks"].([]string)
	for _, expectedTask := range []string{"apt", "systemd", "docker_container", "k8s"} {
		if !slices.Contains(tasks, expectedTask) {
			t.Errorf("expected task module %q in %v", expectedTask, tasks)
		}
	}

	if node.Metadata["image_repo"] != "ghcr.io/pipescope/api" {
		t.Errorf("expected image_repo='ghcr.io/pipescope/api', got %v", node.Metadata["image_repo"])
	}
}
