package parsers

import (
	"os"
	"testing"

	"github.com/pipescope/backend/internal/graph"
)

func TestTerraformParser_SampleMainTF(t *testing.T) {
	var p Parser = NewTerraformParser()

	path := sampleFilePath(t, "terraform/main.tf")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read sample terraform/main.tf: %v", err)
	}

	g, err := p.Parse("terraform/main.tf", content)
	if err != nil {
		t.Fatalf("TerraformParser.Parse failed: %v", err)
	}

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG from TerraformParser, got: %v", err)
	}

	// Verify NodeGroups generated for aws_vpc, aws_eks_cluster, and aws_instance
	groupsByID := make(map[string]graph.NodeGroup, len(g.Groups))
	for _, grp := range g.Groups {
		groupsByID[grp.ID] = grp
	}

	vpcGroup, ok := groupsByID["tf-group-aws-vpc-main"]
	if !ok {
		t.Fatalf("expected VPC NodeGroup 'tf-group-aws-vpc-main', got groups: %+v", g.Groups)
	}
	if vpcGroup.Provider != graph.ProviderAWS || vpcGroup.ResourceType != graph.GroupResourceVPC {
		t.Errorf("unexpected VPC group provider/resource_type: %+v", vpcGroup)
	}

	eksGroup, ok := groupsByID["tf-group-aws-eks-main"]
	if !ok {
		t.Fatalf("expected EKS NodeGroup 'tf-group-aws-eks-main', got groups: %+v", g.Groups)
	}
	if eksGroup.Provider != graph.ProviderAWS || eksGroup.ResourceType != graph.GroupResourceEKSCluster {
		t.Errorf("unexpected EKS group provider/resource_type: %+v", eksGroup)
	}
	if eksGroup.ParentGroupID != "tf-group-aws-vpc-main" {
		t.Errorf("expected EKS group to be nested under VPC group 'tf-group-aws-vpc-main', got %q", eksGroup.ParentGroupID)
	}

	if _, ok := groupsByID["tf-group-aws-instance-app-servers"]; !ok {
		t.Errorf("expected aws_instance NodeGroup 'tf-group-aws-instance-app-servers' to exist")
	}

	// Verify Terraform IaC Nodes
	nodesByID := make(map[string]graph.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		nodesByID[n.ID] = n
	}

	for _, expectedID := range []string{
		"tf-aws-vpc-main",
		"tf-aws-ecr-repository-api",
		"tf-aws-eks-cluster-main",
		"tf-aws-instance-app-servers",
	} {
		n, exists := nodesByID[expectedID]
		if !exists {
			t.Fatalf("expected Terraform node %q to exist", expectedID)
		}
		if n.Stage != graph.StageIaC || n.Tool != graph.ToolTerraform {
			t.Errorf("unexpected stage/tool for %s: %s / %s", expectedID, n.Stage, n.Tool)
		}
	}

	ecrNode := nodesByID["tf-aws-ecr-repository-api"]
	if ecrNode.Metadata["registry_name"] != "pipescope/api" {
		t.Errorf("expected ECR registry_name='pipescope/api', got %v", ecrNode.Metadata["registry_name"])
	}

	// Verify implicit resource reference 'provisions' edges:
	// aws_vpc.main -> aws_eks_cluster.main and aws_vpc.main -> aws_instance.app_servers
	expectedEdges := map[string]bool{
		"tf-aws-vpc-main->tf-aws-eks-cluster-main:provisions":     false,
		"tf-aws-vpc-main->tf-aws-instance-app-servers:provisions": false,
	}
	for _, e := range g.Edges {
		if _, exists := expectedEdges[e.ID]; exists {
			expectedEdges[e.ID] = true
		}
	}
	for edgeID, found := range expectedEdges {
		if !found {
			t.Errorf("expected provisions edge %q not found in %+v", edgeID, g.Edges)
		}
	}
}

func TestArgoCDParser_SampleApplication(t *testing.T) {
	var p Parser = NewArgoCDParser()

	path := sampleFilePath(t, "argocd/application.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read sample argocd/application.yaml: %v", err)
	}

	g, err := p.Parse("argocd/application.yaml", content)
	if err != nil {
		t.Fatalf("ArgoCDParser.Parse failed: %v", err)
	}

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG from ArgoCDParser, got: %v", err)
	}

	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 Argo CD node, got %d", len(g.Nodes))
	}

	n := g.Nodes[0]
	if n.ID != "argocd-app-pipescope-api" {
		t.Errorf("expected node ID 'argocd-app-pipescope-api', got %q", n.ID)
	}
	if n.Stage != graph.StageGitOps || n.Tool != graph.ToolArgoCD {
		t.Errorf("expected stage=gitops, tool=argocd, got %s / %s", n.Stage, n.Tool)
	}
	if n.Metadata["repo_url"] != "https://github.com/pipescope/sample-pipeline.git" {
		t.Errorf("unexpected repo_url: %v", n.Metadata["repo_url"])
	}
	if n.Metadata["sync_path"] != "k8s" {
		t.Errorf("unexpected sync_path: %v", n.Metadata["sync_path"])
	}
	if n.Metadata["target_revision"] != "HEAD" {
		t.Errorf("unexpected target_revision: %v", n.Metadata["target_revision"])
	}
	if n.Metadata["destination_server"] != "https://kubernetes.default.svc" {
		t.Errorf("unexpected destination_server: %v", n.Metadata["destination_server"])
	}
	if n.Metadata["destination_namespace"] != "production" {
		t.Errorf("unexpected destination_namespace: %v", n.Metadata["destination_namespace"])
	}
}

func TestKubernetesParser_SampleDeploymentYAML(t *testing.T) {
	var p Parser = NewKubernetesParser()

	path := sampleFilePath(t, "k8s/deployment.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read sample k8s/deployment.yaml: %v", err)
	}

	g, err := p.Parse("k8s/deployment.yaml", content)
	if err != nil {
		t.Fatalf("KubernetesParser.Parse failed: %v", err)
	}

	if err := g.ValidateDAG(); err != nil {
		t.Fatalf("expected valid DAG from KubernetesParser, got: %v", err)
	}

	// Expect 4 nodes: Deployment (api), Service (api-service), Ingress (api-ingress), StatefulSet (redis-cache)
	if len(g.Nodes) != 4 {
		t.Fatalf("expected 4 Kubernetes nodes, got %d", len(g.Nodes))
	}

	nodesByID := make(map[string]graph.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		nodesByID[n.ID] = n
	}

	deployNode, ok := nodesByID["k8s-deploy-api"]
	if !ok {
		t.Fatalf("expected Deployment node 'k8s-deploy-api' to exist")
	}
	if deployNode.Stage != graph.StageRuntime || deployNode.Tool != graph.ToolKubernetes {
		t.Errorf("unexpected stage/tool for k8s-deploy-api: %s / %s", deployNode.Stage, deployNode.Tool)
	}
	if deployNode.Metadata["container_image"] != "ghcr.io/pipescope/api:latest" {
		t.Errorf("expected container_image='ghcr.io/pipescope/api:latest', got %v", deployNode.Metadata["container_image"])
	}
	if deployNode.Metadata["image_repo"] != "ghcr.io/pipescope/api" {
		t.Errorf("expected image_repo='ghcr.io/pipescope/api', got %v", deployNode.Metadata["image_repo"])
	}

	if _, ok := nodesByID["k8s-sts-redis-cache"]; !ok {
		t.Errorf("expected StatefulSet node 'k8s-sts-redis-cache' to exist")
	}

	// Verify Ingress -> Service -> Deployment deploys_to edges
	expectedEdges := map[string]bool{
		"k8s-ing-api-ingress->k8s-svc-api-service:deploys_to": false,
		"k8s-svc-api-service->k8s-deploy-api:deploys_to":      false,
	}
	for _, e := range g.Edges {
		if _, exists := expectedEdges[e.ID]; exists {
			expectedEdges[e.ID] = true
		}
	}
	for edgeID, found := range expectedEdges {
		if !found {
			t.Errorf("expected deploys_to edge %q not found in %+v", edgeID, g.Edges)
		}
	}
}
