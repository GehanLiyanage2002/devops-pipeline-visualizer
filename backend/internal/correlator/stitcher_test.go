package correlator

import (
	"path/filepath"
	"testing"

	"github.com/pipescope/backend/internal/graph"
)

func TestStitchGraph_SamplePipelineEndToEnd(t *testing.T) {
	sampleDir := filepath.Join("..", "..", "..", "sample-pipeline")

	unified, err := ScanAndStitchDirectory(sampleDir)
	if err != nil {
		t.Fatalf("ScanAndStitchDirectory failed: %v", err)
	}

	if err := unified.ValidateDAG(); err != nil {
		t.Fatalf("expected stitched unified graph to be a valid DAG, got error: %v", err)
	}

	// Verify that all 7 pipeline stages present across the 6 sample config files are represented
	stagesFound := make(map[graph.Stage]bool)
	nodesByID := make(map[string]graph.Node, len(unified.Nodes))
	for _, n := range unified.Nodes {
		stagesFound[n.Stage] = true
		nodesByID[n.ID] = n
	}

	expectedStages := []graph.Stage{
		graph.StageSource,
		graph.StageCI,
		graph.StageConfigMgmt,
		graph.StageArtifact,
		graph.StageIaC,
		graph.StageGitOps,
		graph.StageRuntime,
	}
	for _, st := range expectedStages {
		if !stagesFound[st] {
			t.Errorf("expected stage %q to be present in unified DAG, found stages: %v", st, stagesFound)
		}
	}

	// Verify cross-tool correlated edges across all 5 linkers
	expectedCorrelatedEdges := map[string]graph.RelationType{
		// 1. GitHub Source -> GitHub Actions & GitHub Actions -> Docker image + ECR registry
		"gh-source-repo->gha-job-test:triggers":                  graph.RelationTriggers,
		"gha-job-build->docker-image:builds_image":               graph.RelationBuildsImage,
		"docker-image->tf-aws-ecr-repository-api:pushes_to":      graph.RelationPushesTo,

		// 2. GitHub Actions -> Ansible & Terraform root resources
		"gha-job-provision->ansible-play-configure-app-servers-and-deploy-containers:triggers": graph.RelationTriggers,
		"gha-job-provision->tf-aws-vpc-main:triggers":                                          graph.RelationTriggers,

		// 3. Docker Image -> Kubernetes Deployment
		"docker-image->k8s-deploy-api:pushes_to": graph.RelationPushesTo,

		// 4. Argo CD -> Kubernetes manifests in k8s/
		"argocd-app-pipescope-api->k8s-deploy-api:syncs_manifest":      graph.RelationSyncsManifest,
		"argocd-app-pipescope-api->k8s-svc-api-service:syncs_manifest": graph.RelationSyncsManifest,
		"argocd-app-pipescope-api->k8s-ing-api-ingress:syncs_manifest": graph.RelationSyncsManifest,

		// 5. Ansible -> Terraform EC2 instance & Terraform EKS -> K8s workload
		"ansible-play-configure-app-servers-and-deploy-containers->tf-aws-instance-app-servers:deploys_to": graph.RelationDeploysTo,
		"tf-aws-eks-cluster-main->k8s-deploy-api:provisions":                                               graph.RelationProvisions,
	}

	edgesByID := make(map[string]graph.Edge, len(unified.Edges))
	connectedNodes := make(map[string]bool)
	for _, e := range unified.Edges {
		edgesByID[e.ID] = e
		connectedNodes[e.Source] = true
		connectedNodes[e.Target] = true
	}

	for edgeID, expectedRel := range expectedCorrelatedEdges {
		edge, ok := edgesByID[edgeID]
		if !ok {
			t.Errorf("expected correlated edge %q not found in unified graph", edgeID)
			continue
		}
		if edge.RelationType != expectedRel {
			t.Errorf("expected edge %q to have RelationType %q, got %q", edgeID, expectedRel, edge.RelationType)
		}
	}

	// Verify Kubernetes nodes are nested inside the provisioned EKS cluster NodeGroup
	for _, k8sNodeID := range []string{
		"k8s-deploy-api",
		"k8s-svc-api-service",
		"k8s-ing-api-ingress",
		"k8s-sts-redis-cache",
	} {
		n, ok := nodesByID[k8sNodeID]
		if !ok {
			t.Fatalf("expected Kubernetes node %q to exist", k8sNodeID)
		}
		if n.ParentGroupID != "tf-group-aws-eks-main" {
			t.Errorf("expected Kubernetes node %q to have ParentGroupID 'tf-group-aws-eks-main', got %q", k8sNodeID, n.ParentGroupID)
		}
	}

	// Verify every single node in the unified graph is connected (zero isolated nodes)
	for _, n := range unified.Nodes {
		if !connectedNodes[n.ID] {
			t.Errorf("expected node %q to be connected in the unified DAG, but it has no incoming or outgoing edges", n.ID)
		}
	}
}

func TestApplyManualOverrides(t *testing.T) {
	sampleDir := filepath.Join("..", "..", "..", "sample-pipeline")
	unified, err := ScanAndStitchDirectory(sampleDir)
	if err != nil {
		t.Fatalf("ScanAndStitchDirectory failed: %v", err)
	}

	customNodes := []graph.Node{
		{
			ID:        "manual-cloud-rds",
			Label:     "Production Aurora RDS",
			Stage:     graph.StageCloud,
			Tool:      graph.ToolAWS,
			ZoomLevel: graph.ZoomJobOrService,
			Position:  graph.Position{X: 980, Y: 420},
			Metadata: map[string]interface{}{
				"engine": "aurora-postgresql",
			},
		},
	}

	customEdges := []graph.Edge{
		{
			ID:           "manual-edge-api-rds",
			Source:       "k8s-deploy-api",
			Target:       "manual-cloud-rds",
			RelationType: graph.RelationManualLink,
			Animated:     true,
			Label:        "reads/writes SQL",
		},
	}

	positions := map[string]graph.Position{
		"k8s-deploy-api": {X: 640, Y: 310},
	}

	ApplyManualOverrides(unified, customNodes, customEdges, positions)

	if err := unified.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG after applying manual overrides, got: %v", err)
	}

	var foundManualNode bool
	var deployPos graph.Position
	for _, n := range unified.Nodes {
		if n.ID == "manual-cloud-rds" {
			foundManualNode = true
			if !n.IsManual {
				t.Errorf("expected custom node IsManual=true")
			}
		}
		if n.ID == "k8s-deploy-api" {
			deployPos = n.Position
		}
	}
	if !foundManualNode {
		t.Errorf("expected custom node 'manual-cloud-rds' to be present")
	}
	if deployPos.X != 640 || deployPos.Y != 310 {
		t.Errorf("expected overridden position {640, 310} for k8s-deploy-api, got %+v", deployPos)
	}

	var foundManualEdge bool
	for _, e := range unified.Edges {
		if e.ID == "manual-edge-api-rds" {
			foundManualEdge = true
			if !e.IsManual || e.RelationType != graph.RelationManualLink {
				t.Errorf("unexpected manual edge properties: %+v", e)
			}
		}
	}
	if !foundManualEdge {
		t.Errorf("expected custom edge 'manual-edge-api-rds' to be present")
	}
}
