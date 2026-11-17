package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestNewPipelineGraphAndJSONContract(t *testing.T) {
	g := NewPipelineGraph("pipe-1", "Production E-Commerce Pipeline")

	err := g.AddGroup(NodeGroup{
		ID:           "tf-group-aws-eks-main",
		Label:        "AWS EKS Cluster: prod-cluster",
		Provider:     ProviderAWS,
		ResourceType: GroupResourceEKSCluster,
	})
	if err != nil {
		t.Fatalf("unexpected AddGroup error: %v", err)
	}

	err = g.AddNode(Node{
		ID:            "k8s-deploy-api",
		Label:         "API Deployment",
		Stage:         StageRuntime,
		Tool:          ToolKubernetes,
		Status:        StatusSuccess,
		ZoomLevel:     ZoomJobOrService,
		ParentGroupID: "tf-group-aws-eks-main",
		Position:      Position{X: 420.5, Y: 180.0},
		IsManual:      false,
		Metadata: map[string]interface{}{
			"file_path":       "k8s/deployment.yaml",
			"container_image": "ghcr.io/pipescope/api:latest",
		},
	})
	if err != nil {
		t.Fatalf("unexpected AddNode error: %v", err)
	}

	err = g.AddNode(Node{
		ID:        "argocd-app-api",
		Label:     "Argo CD App: api",
		Stage:     StageGitOps,
		Tool:      ToolArgoCD,
		Status:    StatusSuccess,
		ZoomLevel: ZoomJobOrService,
		Metadata: map[string]interface{}{
			"file_path": "argocd/application.yaml",
			"sync_path": "k8s/",
		},
	})
	if err != nil {
		t.Fatalf("unexpected AddNode error: %v", err)
	}

	err = g.AddEdge(Edge{
		ID:           "edge-argo-k8s",
		Source:       "argocd-app-api",
		Target:       "k8s-deploy-api",
		RelationType: RelationSyncsManifest,
		Animated:     true,
		Label:        "syncs manifest",
		IsManual:     false,
	})
	if err != nil {
		t.Fatalf("unexpected AddEdge error: %v", err)
	}

	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("failed to marshal PipelineGraph: %v", err)
	}

	var rawMap map[string]interface{}
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("failed to unmarshal into map: %v", err)
	}

	for _, requiredKey := range []string{"id", "name", "updated_at", "nodes", "edges", "groups"} {
		if _, ok := rawMap[requiredKey]; !ok {
			t.Errorf("expected top-level JSON key %q to be present", requiredKey)
		}
	}

	var decoded PipelineGraph
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed to unmarshal back into PipelineGraph: %v", err)
	}

	if decoded.ID != "pipe-1" || len(decoded.Nodes) != 2 || len(decoded.Edges) != 1 || len(decoded.Groups) != 1 {
		t.Fatalf("unexpected decoded graph counts: %+v", decoded)
	}
	if decoded.Nodes[0].ParentGroupID != "tf-group-aws-eks-main" {
		t.Errorf("expected ParentGroupID %q, got %q", "tf-group-aws-eks-main", decoded.Nodes[0].ParentGroupID)
	}
	if decoded.Edges[0].RelationType != RelationSyncsManifest {
		t.Errorf("expected RelationType %q, got %q", RelationSyncsManifest, decoded.Edges[0].RelationType)
	}
}

func TestAddValidationAndDefaults(t *testing.T) {
	g := NewPipelineGraph("pipe-defaults", "Defaults & Validation")

	if err := g.AddNode(Node{ID: "   "}); !errors.Is(err, ErrEmptyNodeID) {
		t.Fatalf("expected ErrEmptyNodeID, got %v", err)
	}

	if err := g.AddEdge(Edge{Source: "", Target: "node-b"}); !errors.Is(err, ErrInvalidEdgeEndpoints) {
		t.Fatalf("expected ErrInvalidEdgeEndpoints for empty source, got %v", err)
	}

	if err := g.AddGroup(NodeGroup{ID: ""}); !errors.Is(err, ErrEmptyGroupID) {
		t.Fatalf("expected ErrEmptyGroupID, got %v", err)
	}

	if err := g.AddNode(Node{ID: "node-a", Label: "Node A", Stage: StageSource, Tool: ToolGitHub}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Nodes[0].Status != StatusIdle {
		t.Errorf("expected default StatusIdle, got %q", g.Nodes[0].Status)
	}
	if g.Nodes[0].ZoomLevel != ZoomHighLevelStage {
		t.Errorf("expected default ZoomHighLevelStage (1), got %d", g.Nodes[0].ZoomLevel)
	}
	if g.Nodes[0].Metadata == nil {
		t.Errorf("expected non-nil initialized Metadata map")
	}

	// Auto-generated edge ID and duplicate edge deduplication
	if err := g.AddEdge(Edge{Source: "node-a", Target: "node-b", RelationType: RelationTriggers}); err != nil {
		t.Fatalf("unexpected error adding edge: %v", err)
	}
	if err := g.AddEdge(Edge{Source: "node-a", Target: "node-b", RelationType: RelationTriggers, Label: " triggers CI ", Animated: true}); err != nil {
		t.Fatalf("unexpected error updating duplicate edge: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected duplicate edge to be merged, got %d edges", len(g.Edges))
	}
	if g.Edges[0].ID != "node-a->node-b:triggers" || !g.Edges[0].Animated {
		t.Errorf("unexpected edge after upsert: %+v", g.Edges[0])
	}
}

func TestDeduplicateNodes(t *testing.T) {
	g := NewPipelineGraph("pipe-dedup", "Deduplication Test")

	_ = g.AddNode(Node{
		ID:    "docker-image-api",
		Label: "API Image",
		Stage: StageArtifact,
		Tool:  ToolDocker,
		Metadata: map[string]interface{}{
			"file_path":  "Dockerfile",
			"image_repo": "ghcr.io/pipescope/api",
		},
	})

	_ = g.AddNode(Node{
		ID:    "gha-job-build",
		Label: "Build & Push",
		Stage: StageCI,
		Tool:  ToolGitHubActions,
	})

	// Second occurrence of docker-image-api from another parser with enriched fields
	_ = g.AddNode(Node{
		ID:            "docker-image-api",
		Status:        StatusSuccess,
		ZoomLevel:     ZoomJobOrService,
		ParentGroupID: "group-ecr",
		Position:      Position{X: 250, Y: 120},
		IsManual:      true,
		Metadata: map[string]interface{}{
			"container_image":    "ghcr.io/pipescope/api:sha-9f8e7d",
			"correlation_reason": "Matched image_repo between GitHub Actions and K8s Deployment",
		},
	})

	g.DeduplicateNodes()

	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 deduplicated nodes, got %d", len(g.Nodes))
	}

	// Verify first-seen order is preserved
	if g.Nodes[0].ID != "docker-image-api" || g.Nodes[1].ID != "gha-job-build" {
		t.Fatalf("unexpected node order after deduplication: %s, %s", g.Nodes[0].ID, g.Nodes[1].ID)
	}

	merged := g.Nodes[0]
	if merged.Label != "API Image" || merged.Status != StatusSuccess || merged.ZoomLevel != ZoomJobOrService {
		t.Errorf("scalar fields not properly merged: %+v", merged)
	}
	if merged.ParentGroupID != "group-ecr" || merged.Position.X != 250 || !merged.IsManual {
		t.Errorf("group/position/manual fields not properly merged: %+v", merged)
	}
	if merged.Metadata["file_path"] != "Dockerfile" ||
		merged.Metadata["image_repo"] != "ghcr.io/pipescope/api" ||
		merged.Metadata["container_image"] != "ghcr.io/pipescope/api:sha-9f8e7d" {
		t.Errorf("metadata keys not properly merged: %+v", merged.Metadata)
	}
}

func TestValidateDAG_ValidEndToEndPipeline(t *testing.T) {
	g := NewPipelineGraph("pipe-valid", "Valid Multi-Tool Pipeline")

	_ = g.AddGroup(NodeGroup{
		ID:           "aws-vpc-prod",
		Label:        "AWS Production VPC",
		Provider:     ProviderAWS,
		ResourceType: GroupResourceVPC,
	})
	_ = g.AddGroup(NodeGroup{
		ID:            "aws-eks-prod",
		Label:         "EKS Cluster: prod",
		Provider:      ProviderAWS,
		ResourceType:  GroupResourceEKSCluster,
		ParentGroupID: "aws-vpc-prod",
	})

	nodes := []Node{
		{ID: "gh-repo", Label: "GitHub Repo", Stage: StageSource, Tool: ToolGitHub},
		{ID: "gha-build", Label: "GHA Build", Stage: StageCI, Tool: ToolGitHubActions},
		{ID: "ansible-cfg", Label: "Ansible Provision", Stage: StageConfigMgmt, Tool: ToolAnsible},
		{ID: "docker-img", Label: "Docker Image", Stage: StageArtifact, Tool: ToolDocker},
		{ID: "tf-eks", Label: "Terraform EKS", Stage: StageIaC, Tool: ToolTerraform, ParentGroupID: "aws-vpc-prod"},
		{ID: "argocd-app", Label: "Argo CD App", Stage: StageGitOps, Tool: ToolArgoCD},
		{ID: "k8s-deploy", Label: "K8s Deployment", Stage: StageRuntime, Tool: ToolKubernetes, ParentGroupID: "aws-eks-prod"},
		{ID: "aws-rds", Label: "AWS RDS Postgres", Stage: StageCloud, Tool: ToolAWS, ParentGroupID: "aws-vpc-prod"},
	}
	for _, n := range nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatalf("AddNode(%s) failed: %v", n.ID, err)
		}
	}

	edges := []Edge{
		{Source: "gh-repo", Target: "gha-build", RelationType: RelationTriggers},
		{Source: "gha-build", Target: "ansible-cfg", RelationType: RelationTriggers},
		{Source: "gha-build", Target: "docker-img", RelationType: RelationBuildsImage},
		{Source: "gha-build", Target: "tf-eks", RelationType: RelationTriggers},
		{Source: "tf-eks", Target: "aws-rds", RelationType: RelationProvisions},
		{Source: "docker-img", Target: "argocd-app", RelationType: RelationPushesTo},
		{Source: "argocd-app", Target: "k8s-deploy", RelationType: RelationSyncsManifest},
		{Source: "k8s-deploy", Target: "aws-rds", RelationType: RelationDeploysTo},
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("AddEdge(%s->%s) failed: %v", e.Source, e.Target, err)
		}
	}

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG, got error: %v", err)
	}
}

func TestValidateDAG_CyclesAndOrphans(t *testing.T) {
	t.Run("detects self loop", func(t *testing.T) {
		g := NewPipelineGraph("p1", "Self Loop")
		_ = g.AddNode(Node{ID: "n1", Stage: StageCI, Tool: ToolGitHubActions})
		_ = g.AddEdge(Edge{Source: "n1", Target: "n1", RelationType: RelationTriggers})

		err := g.ValidateDAG()
		if !errors.Is(err, ErrCycleDetected) {
			t.Fatalf("expected ErrCycleDetected for self-loop, got %v", err)
		}
	})

	t.Run("detects multi-hop directed cycle", func(t *testing.T) {
		g := NewPipelineGraph("p2", "Multi-hop Cycle")
		_ = g.AddNode(Node{ID: "source", Stage: StageSource, Tool: ToolGitHub})
		_ = g.AddNode(Node{ID: "ci", Stage: StageCI, Tool: ToolGitHubActions})
		_ = g.AddNode(Node{ID: "artifact", Stage: StageArtifact, Tool: ToolDocker})
		_ = g.AddNode(Node{ID: "runtime", Stage: StageRuntime, Tool: ToolKubernetes})

		_ = g.AddEdge(Edge{Source: "source", Target: "ci", RelationType: RelationTriggers})
		_ = g.AddEdge(Edge{Source: "ci", Target: "artifact", RelationType: RelationBuildsImage})
		_ = g.AddEdge(Edge{Source: "artifact", Target: "runtime", RelationType: RelationDeploysTo})
		// Back-edge creating cycle: runtime -> ci
		_ = g.AddEdge(Edge{Source: "runtime", Target: "ci", RelationType: RelationTriggers})

		err := g.ValidateDAG()
		if !errors.Is(err, ErrCycleDetected) {
			t.Fatalf("expected ErrCycleDetected, got %v", err)
		}
		if !strings.Contains(err.Error(), "ci -> artifact -> runtime -> ci") {
			t.Errorf("expected cycle path in error message, got %q", err.Error())
		}
	})

	t.Run("detects orphaned edge source and target", func(t *testing.T) {
		g := NewPipelineGraph("p3", "Orphaned Edge")
		_ = g.AddNode(Node{ID: "n1", Stage: StageSource, Tool: ToolGitHub})
		_ = g.AddEdge(Edge{Source: "n1", Target: "missing-node", RelationType: RelationTriggers})

		err := g.ValidateDAG()
		if !errors.Is(err, ErrOrphanedEdgeRef) {
			t.Fatalf("expected ErrOrphanedEdgeRef for missing target, got %v", err)
		}

		g2 := NewPipelineGraph("p4", "Orphaned Source")
		_ = g2.AddNode(Node{ID: "n1", Stage: StageSource, Tool: ToolGitHub})
		_ = g2.AddEdge(Edge{Source: "missing-source", Target: "n1", RelationType: RelationTriggers})

		if err := g2.ValidateDAG(); !errors.Is(err, ErrOrphanedEdgeRef) {
			t.Fatalf("expected ErrOrphanedEdgeRef for missing source, got %v", err)
		}
	})

	t.Run("detects orphaned parent group on node and group cycle", func(t *testing.T) {
		g := NewPipelineGraph("p5", "Orphaned Group")
		_ = g.AddNode(Node{
			ID:            "n1",
			Stage:         StageRuntime,
			Tool:          ToolKubernetes,
			ParentGroupID: "non-existent-cluster",
		})

		if err := g.ValidateDAG(); !errors.Is(err, ErrOrphanedGroupRef) {
			t.Fatalf("expected ErrOrphanedGroupRef, got %v", err)
		}

		gCycle := NewPipelineGraph("p6", "Group Cycle")
		_ = gCycle.AddGroup(NodeGroup{ID: "g1", Provider: ProviderAWS, ResourceType: GroupResourceVPC, ParentGroupID: "g2"})
		_ = gCycle.AddGroup(NodeGroup{ID: "g2", Provider: ProviderAWS, ResourceType: GroupResourceEKSCluster, ParentGroupID: "g1"})

		if err := gCycle.ValidateDAG(); !errors.Is(err, ErrCycleDetected) {
			t.Fatalf("expected ErrCycleDetected for cyclic group hierarchy, got %v", err)
		}
	})
}

func TestPipelineGraphConcurrency(t *testing.T) {
	g := NewPipelineGraph("pipe-concurrent", "Concurrent Graph Builder")

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()

			grpID := fmt.Sprintf("group-%d", id%4)
			_ = g.AddGroup(NodeGroup{
				ID:           grpID,
				Label:        fmt.Sprintf("Group %d", id%4),
				Provider:     ProviderAWS,
				ResourceType: GroupResourceVPC,
			})

			srcID := fmt.Sprintf("node-stage1-%d", id%8)
			dstID := fmt.Sprintf("node-stage2-%d", id%8)

			_ = g.AddNode(Node{
				ID:            srcID,
				Label:         "Source Node",
				Stage:         StageCI,
				Tool:          ToolGitHubActions,
				ParentGroupID: grpID,
				Metadata:      map[string]interface{}{"worker": id},
			})
			_ = g.AddNode(Node{
				ID:            dstID,
				Label:         "Target Node",
				Stage:         StageRuntime,
				Tool:          ToolKubernetes,
				ParentGroupID: grpID,
				Metadata:      map[string]interface{}{"worker": id},
			})
			_ = g.AddEdge(Edge{
				Source:       srcID,
				Target:       dstID,
				RelationType: RelationTriggers,
			})

			g.DeduplicateNodes()
			_ = g.ValidateDAG()
		}(i)
	}

	wg.Wait()
	g.DeduplicateNodes()

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG after concurrent operations, got: %v", err)
	}
	if len(g.Nodes) != 16 {
		t.Errorf("expected 16 deduplicated nodes (8 stage1 + 8 stage2), got %d", len(g.Nodes))
	}
}
