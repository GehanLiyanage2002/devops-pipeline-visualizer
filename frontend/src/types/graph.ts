export type Stage =
  | 'source'
  | 'ci'
  | 'config_mgmt'
  | 'artifact'
  | 'iac'
  | 'gitops'
  | 'runtime'
  | 'cloud';

export type Tool =
  | 'github'
  | 'github_actions'
  | 'ansible'
  | 'docker'
  | 'terraform'
  | 'argocd'
  | 'kubernetes'
  | 'aws'
  | 'azure';

export type NodeStatus =
  | 'idle'
  | 'running'
  | 'success'
  | 'failed'
  | 'out_of_sync';

export type RelationType =
  | 'triggers'
  | 'builds_image'
  | 'pushes_to'
  | 'provisions'
  | 'syncs_manifest'
  | 'deploys_to'
  | 'manual_link';

export type GroupProvider = 'aws' | 'azure' | 'kubernetes';

export type GroupResourceType =
  | 'vpc'
  | 'vnet'
  | 'eks_cluster'
  | 'aks_cluster'
  | 'namespace';

export type ZoomLevel = 1 | 2 | 3;

export interface Position {
  x: number;
  y: number;
}

export interface NodeMetadata {
  file_path?: string;
  image_repo?: string;
  container_image?: string;
  target_hosts?: string;
  repo_url?: string;
  sync_path?: string;
  correlation_reason?: string;
  [key: string]: unknown;
}

export interface Node {
  id: string;
  label: string;
  stage: Stage;
  tool: Tool;
  status: NodeStatus;
  zoom_level: number;
  parent_group_id?: string;
  position: Position;
  is_manual: boolean;
  metadata: NodeMetadata;
}

export interface Edge {
  id: string;
  source: string;
  target: string;
  relation_type: RelationType;
  animated: boolean;
  label: string;
  is_manual: boolean;
}

export interface NodeGroup {
  id: string;
  label: string;
  provider: GroupProvider;
  resource_type: GroupResourceType;
  parent_group_id?: string;
}

export interface PipelineGraph {
  id: string;
  name: string;
  updated_at: string;
  nodes: Node[];
  edges: Edge[];
  groups: NodeGroup[];
}

export interface TelemetryEvent {
  type: 'connected' | 'telemetry_update' | 'simulation_completed' | 'graph_updated';
  timestamp: string;
  stage?: string;
  message?: string;
  node_statuses: Record<string, NodeStatus>;
  animated_edges: string[];
  graph?: PipelineGraph;
}
