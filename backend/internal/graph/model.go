package graph

import "sync"

// Stage represents the lifecycle stage of a DevOps node in the pipeline.
type Stage string

const (
	StageSource     Stage = "source"
	StageCI         Stage = "ci"
	StageConfigMgmt Stage = "config_mgmt"
	StageArtifact   Stage = "artifact"
	StageIaC        Stage = "iac"
	StageGitOps     Stage = "gitops"
	StageRuntime    Stage = "runtime"
	StageCloud      Stage = "cloud"
)

// Tool identifies the DevOps or cloud tool associated with a node.
type Tool string

const (
	ToolGitHub        Tool = "github"
	ToolGitHubActions Tool = "github_actions"
	ToolAnsible       Tool = "ansible"
	ToolDocker        Tool = "docker"
	ToolTerraform     Tool = "terraform"
	ToolArgoCD        Tool = "argocd"
	ToolKubernetes    Tool = "kubernetes"
	ToolAWS           Tool = "aws"
	ToolAzure         Tool = "azure"
)

// NodeStatus represents the execution or synchronization state of a node.
type NodeStatus string

const (
	StatusIdle      NodeStatus = "idle"
	StatusRunning   NodeStatus = "running"
	StatusSuccess   NodeStatus = "success"
	StatusFailed    NodeStatus = "failed"
	StatusOutOfSync NodeStatus = "out_of_sync"
)

// ZoomLevel constants define semantic zoom granularity.
const (
	ZoomHighLevelStage   = 1 // 1 = High-Level Stage
	ZoomJobOrService     = 2 // 2 = Job/Service
	ZoomGranularResource = 3 // 3 = Granular Resource
)

// RelationType defines the semantic relationship of a directed edge.
type RelationType string

const (
	RelationTriggers      RelationType = "triggers"
	RelationBuildsImage   RelationType = "builds_image"
	RelationPushesTo      RelationType = "pushes_to"
	RelationProvisions    RelationType = "provisions"
	RelationSyncsManifest RelationType = "syncs_manifest"
	RelationDeploysTo     RelationType = "deploys_to"
	RelationManualLink    RelationType = "manual_link"
)

// GroupProvider identifies the cloud or container platform of a NodeGroup.
type GroupProvider string

const (
	ProviderAWS        GroupProvider = "aws"
	ProviderAzure      GroupProvider = "azure"
	ProviderKubernetes GroupProvider = "kubernetes"
)

// GroupResourceType identifies the boundary type of a NodeGroup.
type GroupResourceType string

const (
	GroupResourceVPC        GroupResourceType = "vpc"
	GroupResourceVNet       GroupResourceType = "vnet"
	GroupResourceEKSCluster GroupResourceType = "eks_cluster"
	GroupResourceAKSCluster GroupResourceType = "aks_cluster"
	GroupResourceNamespace  GroupResourceType = "namespace"
)

// Position holds 2D canvas coordinates for manual layout overrides.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// NodeGroup represents a logical boundary container (VPC, Cloud Account, K8s Cluster, or Namespace).
type NodeGroup struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	Provider      GroupProvider     `json:"provider"`
	ResourceType  GroupResourceType `json:"resource_type"`
	ParentGroupID string            `json:"parent_group_id,omitempty"`
}

// Node represents a single vertex in the pipeline DAG.
type Node struct {
	ID            string                 `json:"id"`
	Label         string                 `json:"label"`
	Stage         Stage                  `json:"stage"`
	Tool          Tool                   `json:"tool"`
	Status        NodeStatus             `json:"status"`
	ZoomLevel     int                    `json:"zoom_level"`
	ParentGroupID string                 `json:"parent_group_id,omitempty"`
	Position      Position               `json:"position"`
	IsManual      bool                   `json:"is_manual"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// Edge represents a directed relationship between two nodes in the DAG.
type Edge struct {
	ID           string       `json:"id"`
	Source       string       `json:"source"`
	Target       string       `json:"target"`
	RelationType RelationType `json:"relation_type"`
	Animated     bool         `json:"animated"`
	Label        string       `json:"label"`
	IsManual     bool         `json:"is_manual"`
}

// PipelineGraph is the root Unified Directed Acyclic Graph (DAG) serialized to JSON.
type PipelineGraph struct {
	mu        sync.RWMutex `json:"-"`
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	UpdatedAt string       `json:"updated_at"`
	Nodes     []Node       `json:"nodes"`
	Edges     []Edge       `json:"edges"`
	Groups    []NodeGroup  `json:"groups"`
}
