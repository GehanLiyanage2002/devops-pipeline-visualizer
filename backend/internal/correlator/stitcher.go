package correlator

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pipescope/backend/internal/graph"
	"github.com/pipescope/backend/internal/parsers"
)

// StitchGraph merges partial PipelineGraphs discovered across a repository/workspace
// and runs cross-tool heuristic linkers to produce a single unified DAG.
func StitchGraph(partials []*graph.PipelineGraph) *graph.PipelineGraph {
	unified := graph.NewPipelineGraph("pipescope-unified", "Unified DevOps Pipeline")

	for _, part := range partials {
		if part == nil {
			continue
		}
		for _, grp := range part.Groups {
			_ = unified.AddGroup(grp)
		}
		for _, node := range part.Nodes {
			_ = unified.AddNode(node)
		}
		for _, edge := range part.Edges {
			_ = unified.AddEdge(edge)
		}
	}

	unified.DeduplicateNodes()

	linkGitHubActionsAndDocker(unified)
	linkGitHubActionsToAnsibleAndTerraform(unified)
	linkDockerToKubernetes(unified)
	linkArgoCDToGitAndKubernetes(unified)
	linkTerraformCloudToKubernetesAndAnsible(unified)

	return unified
}

// ApplyManualOverrides applies user-defined custom nodes, custom edges, and manual
// canvas node positions onto a PipelineGraph so manual architecture edits survive re-parsing.
func ApplyManualOverrides(
	g *graph.PipelineGraph,
	customNodes []graph.Node,
	customEdges []graph.Edge,
	nodePositions map[string]graph.Position,
) {
	if g == nil {
		return
	}

	for _, cn := range customNodes {
		cn.IsManual = true
		if cn.Metadata == nil {
			cn.Metadata = make(map[string]interface{})
		}
		_ = g.AddNode(cn)
	}
	g.DeduplicateNodes()

	nodeSet := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		nodeSet[n.ID] = true
	}

	for _, ce := range customEdges {
		if !nodeSet[ce.Source] || !nodeSet[ce.Target] || ce.Source == ce.Target {
			continue
		}
		ce.IsManual = true
		if ce.RelationType == "" {
			ce.RelationType = graph.RelationManualLink
		}
		if ce.Label == "" {
			ce.Label = "manual link"
		}
		_ = g.AddEdge(ce)
	}

	if len(nodePositions) > 0 {
		for i := range g.Nodes {
			if pos, ok := nodePositions[g.Nodes[i].ID]; ok {
				g.Nodes[i].Position = pos
			}
		}
	}
}

// FileInput represents an in-memory configuration file path and its raw content.
type FileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ParseSingleFile dispatches a single file to the matching static parser based on its path/content.
func ParseSingleFile(filePath string, content []byte) (*graph.PipelineGraph, error) {
	normPath := normalizePath(filePath)
	lowerPath := strings.ToLower(normPath)
	baseName := strings.ToLower(filepath.Base(normPath))

	switch {
	case strings.Contains(lowerPath, ".github/workflows/") && (strings.HasSuffix(lowerPath, ".yml") || strings.HasSuffix(lowerPath, ".yaml")):
		return parsers.NewGitHubActionsParser().Parse(normPath, content)

	case baseName == "dockerfile" || strings.HasPrefix(baseName, "dockerfile.") || strings.HasSuffix(baseName, ".dockerfile"):
		return parsers.NewDockerfileParser().Parse(normPath, content)

	case (strings.Contains(lowerPath, "ansible/") || baseName == "playbook.yml" || baseName == "playbook.yaml" || baseName == "site.yml" || baseName == "site.yaml") &&
		(strings.HasSuffix(lowerPath, ".yml") || strings.HasSuffix(lowerPath, ".yaml")):
		return parsers.NewAnsibleParser().Parse(normPath, content)

	case strings.HasSuffix(lowerPath, ".tf") || strings.HasSuffix(lowerPath, ".tfstate"):
		return parsers.NewTerraformParser().Parse(normPath, content)

	case strings.HasSuffix(lowerPath, ".yml") || strings.HasSuffix(lowerPath, ".yaml"):
		rawStr := string(content)
		if strings.Contains(rawStr, "argoproj.io/") && strings.Contains(rawStr, "kind: Application") {
			return parsers.NewArgoCDParser().Parse(normPath, content)
		} else if strings.Contains(rawStr, "apiVersion:") && strings.Contains(rawStr, "kind:") {
			return parsers.NewKubernetesParser().Parse(normPath, content)
		}
	}

	return nil, nil
}

// ParseAndStitchFiles parses a slice of in-memory files and stitches their partial graphs into a unified DAG.
func ParseAndStitchFiles(files []FileInput) (*graph.PipelineGraph, error) {
	var partials []*graph.PipelineGraph
	for _, f := range files {
		if strings.TrimSpace(f.Path) == "" {
			continue
		}
		part, err := ParseSingleFile(f.Path, []byte(f.Content))
		if err != nil {
			return nil, err
		}
		if part != nil {
			partials = append(partials, part)
		}
	}
	return StitchGraph(partials), nil
}

// ScanAndStitchDirectory walks a repository or sample-pipeline directory, runs the appropriate
// static parser on each recognized DevOps config file, and stitches the partial graphs together.
func ScanAndStitchDirectory(rootDir string) (*graph.PipelineGraph, error) {
	var partials []*graph.PipelineGraph

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".terraform" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			relPath = path
		}
		normPath := filepath.ToSlash(relPath)

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		part, parseErr := ParseSingleFile(normPath, content)
		if parseErr != nil {
			return parseErr
		}
		if part != nil {
			partials = append(partials, part)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return StitchGraph(partials), nil
}

// 1. GitHub Actions <-> Dockerfile & Container Registry Linker
func linkGitHubActionsAndDocker(g *graph.PipelineGraph) {
	var sourceNodeID string
	for _, n := range g.Nodes {
		if n.Stage == graph.StageSource && n.Tool == graph.ToolGitHub {
			sourceNodeID = n.ID
			break
		}
	}

	// Track which github_actions jobs already have incoming edges from github or github_actions
	hasIncomingCI := make(map[string]bool)
	nodeByID := make(map[string]*graph.Node, len(g.Nodes))
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	for _, e := range g.Edges {
		if src, ok := nodeByID[e.Source]; ok {
			if src.Tool == graph.ToolGitHub || src.Tool == graph.ToolGitHubActions {
				hasIncomingCI[e.Target] = true
			}
		}
	}

	// Connect github commit trigger node to initial github_actions job nodes
	if sourceNodeID != "" {
		for _, n := range g.Nodes {
			if n.Tool == graph.ToolGitHubActions && !hasIncomingCI[n.ID] {
				_ = g.AddEdge(graph.Edge{
					Source:       sourceNodeID,
					Target:       n.ID,
					RelationType: graph.RelationTriggers,
					Label:        "triggers",
				})
			}
		}
	}

	// Find Docker artifact nodes and Terraform container registry nodes
	var dockerIndices []int
	var registryIndices []int
	for i, n := range g.Nodes {
		if n.Tool == graph.ToolDocker && n.Stage == graph.StageArtifact {
			dockerIndices = append(dockerIndices, i)
		}
		if n.Tool == graph.ToolTerraform && stringMeta(n.Metadata, "registry_name") != "" {
			registryIndices = append(registryIndices, i)
		}
	}

	// Connect GitHub Actions jobs that build/push images to the Docker Artifact Node
	for i := range g.Nodes {
		jobNode := &g.Nodes[i]
		if jobNode.Tool != graph.ToolGitHubActions {
			continue
		}
		imgRepo := stringMeta(jobNode.Metadata, "image_repo")
		containerImg := stringMeta(jobNode.Metadata, "container_image")
		if imgRepo == "" && containerImg == "" {
			continue
		}

		for _, dIdx := range dockerIndices {
			dockerNode := &g.Nodes[dIdx]
			if stringMeta(dockerNode.Metadata, "image_repo") == "" && imgRepo != "" {
				dockerNode.Metadata["image_repo"] = imgRepo
			}
			if stringMeta(dockerNode.Metadata, "container_image") == "" && containerImg != "" {
				dockerNode.Metadata["container_image"] = containerImg
			}
			dockerNode.Metadata["correlation_reason"] = fmt.Sprintf(
				"Linked to GitHub Actions job %q building image repo %q",
				jobNode.ID, imgRepo,
			)

			_ = g.AddEdge(graph.Edge{
				Source:       jobNode.ID,
				Target:       dockerNode.ID,
				RelationType: graph.RelationBuildsImage,
				Label:        "builds image",
			})

			// Also link Docker image node to matching container registry (ECR/ACR) if present
			for _, rIdx := range registryIndices {
				regNode := &g.Nodes[rIdx]
				regName := stringMeta(regNode.Metadata, "registry_name")
				if imageReposMatch(imgRepo, regName) {
					regNode.Metadata["correlation_reason"] = fmt.Sprintf(
						"Matched container registry %q with image repo %q",
						regName, imgRepo,
					)
					_ = g.AddEdge(graph.Edge{
						Source:       dockerNode.ID,
						Target:       regNode.ID,
						RelationType: graph.RelationPushesTo,
						Label:        "pushes to registry",
					})
				}
			}
		}
	}
}

// 2. GitHub Actions <-> Ansible / Terraform Linker
func linkGitHubActionsToAnsibleAndTerraform(g *graph.PipelineGraph) {
	// Identify root Terraform nodes (Terraform nodes that have no incoming provisions edges from other Terraform nodes)
	hasIncomingTFProvision := make(map[string]bool)
	for _, e := range g.Edges {
		if e.RelationType == graph.RelationProvisions && strings.HasPrefix(e.Source, "tf-") && strings.HasPrefix(e.Target, "tf-") {
			hasIncomingTFProvision[e.Target] = true
		}
	}

	for i := range g.Nodes {
		jobNode := &g.Nodes[i]
		if jobNode.Tool != graph.ToolGitHubActions {
			continue
		}

		if boolMeta(jobNode.Metadata, "executes_ansible") {
			playbookPath := normalizePath(stringMeta(jobNode.Metadata, "ansible_playbook"))
			for j := range g.Nodes {
				ansNode := &g.Nodes[j]
				if ansNode.Tool != graph.ToolAnsible {
					continue
				}
				ansFile := normalizePath(stringMeta(ansNode.Metadata, "file_path"))
				if playbookPath == "" || ansFile == playbookPath || strings.HasSuffix(ansFile, playbookPath) || strings.HasSuffix(playbookPath, ansFile) {
					ansNode.Metadata["correlation_reason"] = fmt.Sprintf(
						"Triggered by GitHub Actions job %q running ansible-playbook",
						jobNode.ID,
					)
					_ = g.AddEdge(graph.Edge{
						Source:       jobNode.ID,
						Target:       ansNode.ID,
						RelationType: graph.RelationTriggers,
						Label:        "runs playbook",
					})
				}
			}
		}

		if boolMeta(jobNode.Metadata, "executes_terraform") {
			for j := range g.Nodes {
				tfNode := &g.Nodes[j]
				if tfNode.Tool != graph.ToolTerraform {
					continue
				}
				if !hasIncomingTFProvision[tfNode.ID] {
					tfNode.Metadata["correlation_reason"] = fmt.Sprintf(
						"Triggered by GitHub Actions job %q executing terraform apply",
						jobNode.ID,
					)
					_ = g.AddEdge(graph.Edge{
						Source:       jobNode.ID,
						Target:       tfNode.ID,
						RelationType: graph.RelationTriggers,
						Label:        "terraform apply",
					})
				}
			}
		}
	}
}

// 3. Docker Image <-> Kubernetes Deployment Linker
func linkDockerToKubernetes(g *graph.PipelineGraph) {
	for i := range g.Nodes {
		srcNode := &g.Nodes[i]
		if srcNode.Tool != graph.ToolDocker {
			continue
		}
		dockerRepo := stringMeta(srcNode.Metadata, "image_repo")
		dockerImg := stringMeta(srcNode.Metadata, "container_image")
		if dockerRepo == "" && dockerImg == "" {
			continue
		}

		for j := range g.Nodes {
			k8sNode := &g.Nodes[j]
			if k8sNode.Tool != graph.ToolKubernetes {
				continue
			}
			k8sRepo := stringMeta(k8sNode.Metadata, "image_repo")
			k8sImg := stringMeta(k8sNode.Metadata, "container_image")
			if k8sRepo == "" && k8sImg == "" {
				continue
			}

			if imageReposMatch(dockerRepo, k8sRepo) || (dockerImg != "" && dockerImg == k8sImg) {
				k8sNode.Metadata["correlation_reason"] = fmt.Sprintf(
					"Matched container image %q with Docker artifact %q",
					k8sImg, srcNode.ID,
				)
				_ = g.AddEdge(graph.Edge{
					Source:       srcNode.ID,
					Target:       k8sNode.ID,
					RelationType: graph.RelationPushesTo,
					Label:        "pushes image to",
				})
			}
		}
	}
}

// 4. Argo CD <-> Git Repo & Kubernetes Resources Linker
func linkArgoCDToGitAndKubernetes(g *graph.PipelineGraph) {
	var sourceNodeID string
	for _, n := range g.Nodes {
		if n.Stage == graph.StageSource && n.Tool == graph.ToolGitHub {
			sourceNodeID = n.ID
			break
		}
	}

	for i := range g.Nodes {
		argoNode := &g.Nodes[i]
		if argoNode.Tool != graph.ToolArgoCD {
			continue
		}

		if sourceNodeID != "" && stringMeta(argoNode.Metadata, "repo_url") != "" {
			_ = g.AddEdge(graph.Edge{
				Source:       sourceNodeID,
				Target:       argoNode.ID,
				RelationType: graph.RelationTriggers,
				Label:        "watches repo",
			})
		}

		syncPath := strings.Trim(normalizePath(stringMeta(argoNode.Metadata, "sync_path")), "/")
		if syncPath == "" {
			continue
		}

		for j := range g.Nodes {
			k8sNode := &g.Nodes[j]
			if k8sNode.Tool != graph.ToolKubernetes {
				continue
			}
			k8sFile := normalizePath(stringMeta(k8sNode.Metadata, "file_path"))
			k8sDir := strings.Trim(filepath.ToSlash(filepath.Dir(k8sFile)), "/")

			if k8sDir == syncPath || strings.HasPrefix(k8sFile, syncPath+"/") || syncPath == "." {
				if stringMeta(k8sNode.Metadata, "correlation_reason") == "" {
					k8sNode.Metadata["correlation_reason"] = fmt.Sprintf(
						"Synced by Argo CD Application %q from path %q",
						argoNode.ID, syncPath,
					)
				}
				_ = g.AddEdge(graph.Edge{
					Source:       argoNode.ID,
					Target:       k8sNode.ID,
					RelationType: graph.RelationSyncsManifest,
					Label:        "syncs manifest",
				})
			}
		}
	}
}

// 5. Terraform Cloud Infrastructure <-> Kubernetes & Ansible Nesting
func linkTerraformCloudToKubernetesAndAnsible(g *graph.PipelineGraph) {
	// Find provisioned EKS or AKS cluster NodeGroup
	var clusterGroupID string
	for _, grp := range g.Groups {
		if grp.ResourceType == graph.GroupResourceEKSCluster || grp.ResourceType == graph.GroupResourceAKSCluster {
			clusterGroupID = grp.ID
			break
		}
	}

	// Find corresponding EKS or AKS Terraform resource node
	var clusterTFNodeID string
	for _, n := range g.Nodes {
		if n.Tool == graph.ToolTerraform {
			resType := stringMeta(n.Metadata, "resource_type")
			if resType == "aws_eks_cluster" || resType == "azurerm_kubernetes_cluster" {
				clusterTFNodeID = n.ID
				break
			}
		}
	}

	// Nest Kubernetes Deployment/Service/Ingress/StatefulSet nodes inside the cluster NodeGroup
	if clusterGroupID != "" {
		for i := range g.Nodes {
			if g.Nodes[i].Tool == graph.ToolKubernetes && g.Nodes[i].ParentGroupID == "" {
				g.Nodes[i].ParentGroupID = clusterGroupID
			}
		}
	}

	// Connect the Terraform cluster node to Kubernetes workloads (Deployment / StatefulSet)
	if clusterTFNodeID != "" {
		for i := range g.Nodes {
			if g.Nodes[i].Tool == graph.ToolKubernetes {
				kind := stringMeta(g.Nodes[i].Metadata, "kind")
				if kind == "Deployment" || kind == "StatefulSet" {
					_ = g.AddEdge(graph.Edge{
						Source:       clusterTFNodeID,
						Target:       g.Nodes[i].ID,
						RelationType: graph.RelationProvisions,
						Label:        "hosts workload",
					})
				}
			}
		}
	}

	// Link Ansible playbook nodes to aws_instance / VM Terraform nodes when target_hosts match
	for i := range g.Nodes {
		ansNode := &g.Nodes[i]
		if ansNode.Tool != graph.ToolAnsible {
			continue
		}
		ansTarget := strings.ToLower(strings.TrimSpace(stringMeta(ansNode.Metadata, "target_hosts")))
		if ansTarget == "" {
			continue
		}

		for j := range g.Nodes {
			tfNode := &g.Nodes[j]
			if tfNode.Tool != graph.ToolTerraform {
				continue
			}
			tfTarget := strings.ToLower(strings.TrimSpace(stringMeta(tfNode.Metadata, "target_hosts")))
			tfResName := strings.ToLower(strings.TrimSpace(stringMeta(tfNode.Metadata, "resource_name")))

			if (tfTarget != "" && (tfTarget == ansTarget || strings.Contains(ansTarget, tfTarget))) ||
				(tfResName != "" && tfResName == ansTarget) {
				tfNode.Metadata["correlation_reason"] = fmt.Sprintf(
					"Configured by Ansible play %q targeting host group %q",
					ansNode.ID, ansTarget,
				)
				_ = g.AddEdge(graph.Edge{
					Source:       ansNode.ID,
					Target:       tfNode.ID,
					RelationType: graph.RelationDeploysTo,
					Label:        "configures hosts",
				})
			}
		}
	}
}

func stringMeta(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func boolMeta(meta map[string]interface{}, key string) bool {
	if meta == nil {
		return false
	}
	if v, ok := meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func normalizePath(p string) string {
	cleaned := filepath.ToSlash(strings.TrimSpace(p))
	cleaned = strings.TrimPrefix(cleaned, "./")
	return cleaned
}

func imageReposMatch(repoA, repoB string) bool {
	a := strings.ToLower(strings.TrimSpace(repoA))
	b := strings.ToLower(strings.TrimSpace(repoB))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}
