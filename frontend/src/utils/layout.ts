import ELK, { type ElkNode, type ElkExtendedEdge } from 'elkjs/lib/elk.bundled.js';
import {
  MarkerType,
  type Edge as RFEdge,
  type Node as RFNode,
} from '@xyflow/react';
import type {
  Edge as PipelineEdge,
  Node as PipelineNode,
  NodeGroup,
  NodeStatus,
  PipelineGraph,
  RelationType,
  Stage,
  Tool,
  ZoomLevel,
} from '../types/graph';

const elk = new ELK();

const NODE_WIDTH = 288;
const NODE_HEIGHT = 138;

export interface DevOpsNodeData extends Record<string, unknown> {
  node: PipelineNode;
}

export interface CloudGroupNodeData extends Record<string, unknown> {
  group: NodeGroup;
}

const STAGE_METADATA: Record<
  Stage,
  { label: string; defaultTool: Tool; order: number }
> = {
  source: { label: 'Source Control Stage', defaultTool: 'github', order: 1 },
  ci: { label: 'Continuous Integration (CI)', defaultTool: 'github_actions', order: 2 },
  artifact: { label: 'Container & Artifact Stage', defaultTool: 'docker', order: 3 },
  iac: { label: 'Infrastructure as Code (IaC)', defaultTool: 'terraform', order: 4 },
  config_mgmt: { label: 'Configuration Management', defaultTool: 'ansible', order: 5 },
  gitops: { label: 'GitOps Sync & Delivery', defaultTool: 'argocd', order: 6 },
  runtime: { label: 'Kubernetes Runtime Workloads', defaultTool: 'kubernetes', order: 7 },
  cloud: { label: 'Cloud Platform Services', defaultTool: 'aws', order: 8 },
};

export const RELATION_COLORS: Record<RelationType, string> = {
  triggers: '#38bdf8',       // sky-400
  builds_image: '#818cf8',   // indigo-400
  pushes_to: '#c084fc',      // purple-400
  provisions: '#fbbf24',     // amber-400
  syncs_manifest: '#34d399', // emerald-400
  deploys_to: '#2dd4bf',     // teal-400
  manual_link: '#f472b6',    // pink-400
};

export function rollUpStatus(statuses: NodeStatus[]): NodeStatus {
  if (statuses.includes('running')) return 'running';
  if (statuses.includes('failed')) return 'failed';
  if (statuses.includes('out_of_sync')) return 'out_of_sync';
  if (statuses.includes('success')) return 'success';
  return 'idle';
}

/**
 * Computes the transitive upstream (ancestors) and downstream (descendants / blast radius)
 * node IDs and edge IDs for a given selectedNodeId.
 */
export function computeBlastRadius(
  selectedNodeId: string | null,
  edges: { id: string; source: string; target: string }[]
): {
  upstreamNodes: Set<string>;
  downstreamNodes: Set<string>;
  connectedEdges: Set<string>;
} {
  const upstreamNodes = new Set<string>();
  const downstreamNodes = new Set<string>();
  const connectedEdges = new Set<string>();

  if (!selectedNodeId) {
    return { upstreamNodes, downstreamNodes, connectedEdges };
  }

  // BFS downstream (blast radius)
  const downQueue = [selectedNodeId];
  const seenDown = new Set<string>([selectedNodeId]);
  while (downQueue.length > 0) {
    const curr = downQueue.shift()!;
    for (const e of edges) {
      if (e.source === curr) {
        connectedEdges.add(e.id);
        if (!seenDown.has(e.target)) {
          seenDown.add(e.target);
          downstreamNodes.add(e.target);
          downQueue.push(e.target);
        }
      }
    }
  }

  // BFS upstream (trigger chain)
  const upQueue = [selectedNodeId];
  const seenUp = new Set<string>([selectedNodeId]);
  while (upQueue.length > 0) {
    const curr = upQueue.shift()!;
    for (const e of edges) {
      if (e.target === curr) {
        connectedEdges.add(e.id);
        if (!seenUp.has(e.source)) {
          seenUp.add(e.source);
          upstreamNodes.add(e.source);
          upQueue.push(e.source);
        }
      }
    }
  }

  return { upstreamNodes, downstreamNodes, connectedEdges };
}

/**
 * Normalizes single-instance Terraform compute wrappers (e.g. tf-group-aws-instance-*)
 * so the instance node sits directly inside its parent VPC group without an extra redundant
 * bounding box around a single node.
 */
function normalizeGroupsForVisualClarity(
  nodes: PipelineNode[],
  groups: NodeGroup[]
): { nodes: PipelineNode[]; groups: NodeGroup[] } {
  const groupById = new Map(groups.map((g) => [g.id, g]));
  const flattenedGroupIds = new Set(
    groups
      .filter((g) => g.id.startsWith('tf-group-aws-instance-'))
      .map((g) => g.id)
  );

  if (flattenedGroupIds.size === 0) {
    return { nodes, groups };
  }

  const normalizedNodes = nodes.map((n) => {
    if (n.parent_group_id && flattenedGroupIds.has(n.parent_group_id)) {
      const wrapperGrp = groupById.get(n.parent_group_id);
      return {
        ...n,
        parent_group_id: wrapperGrp?.parent_group_id || undefined,
      };
    }
    return n;
  });

  const normalizedGroups = groups.filter((g) => !flattenedGroupIds.has(g.id));
  return { nodes: normalizedNodes, groups: normalizedGroups };
}

/**
 * Projects a full PipelineGraph into the requested semantic zoom granularity:
 * - Level 1: High-level Executive Stage Flow (collapses granular resources into stage nodes)
 * - Level 2: Jobs & Services (filters out zoom_level 3 granular proxies)
 * - Level 3: Full Granular Infrastructure (shows all nodes and nested cloud groups)
 */
export function filterGraphByZoomLevel(
  graph: PipelineGraph,
  zoomLevel: ZoomLevel
): { nodes: PipelineNode[]; edges: PipelineEdge[]; groups: NodeGroup[] } {
  const normalized = normalizeGroupsForVisualClarity(graph.nodes, graph.groups);

  if (zoomLevel === 1) {
    const byStage = new Map<Stage, PipelineNode[]>();
    for (const node of normalized.nodes) {
      const list = byStage.get(node.stage) ?? [];
      list.push(node);
      byStage.set(node.stage, list);
    }

    const stageNodes: PipelineNode[] = [];
    const nodeToStageID = new Map<string, string>();

    const sortedStages = Array.from(byStage.keys()).sort(
      (a, b) => (STAGE_METADATA[a]?.order ?? 99) - (STAGE_METADATA[b]?.order ?? 99)
    );

    for (const stage of sortedStages) {
      const members = byStage.get(stage) ?? [];
      const stageNodeID = `stage-summary-${stage}`;
      for (const m of members) {
        nodeToStageID.set(m.id, stageNodeID);
      }

      const metaInfo = STAGE_METADATA[stage] ?? {
        label: stage.toUpperCase(),
        defaultTool: members[0]?.tool ?? 'github',
      };

      const tools = Array.from(new Set(members.map((m) => m.tool)));
      stageNodes.push({
        id: stageNodeID,
        label: metaInfo.label,
        stage,
        tool: members[0]?.tool ?? metaInfo.defaultTool,
        status: rollUpStatus(members.map((m) => m.status)),
        zoom_level: 1,
        position: { x: 0, y: 0 },
        is_manual: false,
        metadata: {
          file_path: `${members.length} resources`,
          member_count: members.length,
          member_ids: members.map((m) => m.id),
          member_labels: members.map((m) => m.label),
          tools,
          correlation_reason: `Executive stage summary aggregating ${members.length} ${stage} resource(s): ${members
            .map((m) => m.label)
            .join(', ')}`,
        },
      });
    }

    const stageEdgesMap = new Map<string, PipelineEdge>();
    for (const edge of graph.edges) {
      const srcStageID = nodeToStageID.get(edge.source);
      const tgtStageID = nodeToStageID.get(edge.target);
      if (!srcStageID || !tgtStageID || srcStageID === tgtStageID) continue;

      const id = `${srcStageID}->${tgtStageID}`;
      const existing = stageEdgesMap.get(id);
      if (existing) {
        existing.animated = existing.animated || edge.animated;
      } else {
        stageEdgesMap.set(id, {
          id,
          source: srcStageID,
          target: tgtStageID,
          relation_type: edge.relation_type,
          animated: edge.animated,
          label: edge.relation_type.replace(/_/g, ' '),
          is_manual: false,
        });
      }
    }

    return {
      nodes: stageNodes,
      edges: Array.from(stageEdgesMap.values()),
      groups: [],
    };
  }

  if (zoomLevel === 2) {
    const visibleNodes = normalized.nodes.filter(
      (n) => n.zoom_level <= 2 || n.is_manual
    );
    const visibleIDs = new Set(visibleNodes.map((n) => n.id));
    const visibleEdges = graph.edges.filter(
      (e) => visibleIDs.has(e.source) && visibleIDs.has(e.target)
    );
    return {
      nodes: visibleNodes,
      edges: visibleEdges,
      groups: normalized.groups,
    };
  }

  const visibleIDs = new Set(normalized.nodes.map((n) => n.id));
  const validEdges = graph.edges.filter(
    (e) => visibleIDs.has(e.source) && visibleIDs.has(e.target) && e.source !== e.target
  );

  return {
    nodes: normalized.nodes,
    edges: validEdges,
    groups: normalized.groups,
  };
}

function hasManualCoordinates(node: PipelineNode): boolean {
  return (
    typeof node.position?.x === 'number' &&
    typeof node.position?.y === 'number' &&
    (node.position.x !== 0 || node.position.y !== 0)
  );
}

/**
 * Computes an automatic hierarchical Left-to-Right ('RIGHT') DAG layout using ELKjs,
 * respecting compound NodeGroup containers (`parent_group_id`) and preserving user-saved
 * manual node positions.
 */
export async function computePipelineLayout(
  graph: PipelineGraph,
  zoomLevel: ZoomLevel
): Promise<{ nodes: RFNode[]; edges: RFEdge[] }> {
  const filtered = filterGraphByZoomLevel(graph, zoomLevel);

  const validGroupIDs = new Set(filtered.groups.map((g) => g.id));
  const usedGroupIDs = new Set<string>();

  for (const n of filtered.nodes) {
    let currGroup = n.parent_group_id;
    while (currGroup && validGroupIDs.has(currGroup)) {
      usedGroupIDs.add(currGroup);
      const grpObj = filtered.groups.find((g) => g.id === currGroup);
      currGroup = grpObj?.parent_group_id;
    }
  }

  const activeGroups = filtered.groups.filter((g) => usedGroupIDs.has(g.id));
  const activeGroupIDs = new Set(activeGroups.map((g) => g.id));

  // Build ELK compound hierarchy
  const elkGroupsMap = new Map<string, ElkNode>();
  for (const grp of activeGroups) {
    elkGroupsMap.set(grp.id, {
      id: grp.id,
      layoutOptions: {
        'elk.algorithm': 'layered',
        'elk.direction': 'RIGHT',
        'elk.padding': '[top=62,left=34,bottom=34,right=34]',
        'elk.spacing.nodeNode': '48',
        'elk.layered.spacing.nodeNodeBetweenLayers': '88',
      },
      children: [],
    });
  }

  const rootChildren: ElkNode[] = [];

  for (const grp of activeGroups) {
    const elkGrp = elkGroupsMap.get(grp.id)!;
    if (grp.parent_group_id && activeGroupIDs.has(grp.parent_group_id)) {
      elkGroupsMap.get(grp.parent_group_id)!.children!.push(elkGrp);
    } else {
      rootChildren.push(elkGrp);
    }
  }

  for (const n of filtered.nodes) {
    const elkLeaf: ElkNode = {
      id: n.id,
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    };
    if (n.parent_group_id && activeGroupIDs.has(n.parent_group_id)) {
      elkGroupsMap.get(n.parent_group_id)!.children!.push(elkLeaf);
    } else {
      rootChildren.push(elkLeaf);
    }
  }

  const elkEdges: ElkExtendedEdge[] = filtered.edges.map((e) => ({
    id: e.id,
    sources: [e.source],
    targets: [e.target],
  }));

  const rootElkGraph: ElkNode = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'RIGHT',
      'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
      'elk.spacing.nodeNode': '56',
      'elk.layered.spacing.nodeNodeBetweenLayers': '114',
      'elk.edgeRouting': 'ORTHOGONAL',
    },
    children: rootChildren,
    edges: elkEdges,
  };

  let laidOutGraph: ElkNode;
  try {
    laidOutGraph = await elk.layout(rootElkGraph);
  } catch {
    laidOutGraph = rootElkGraph;
  }

  const elkCoords = new Map<
    string,
    { x: number; y: number; width: number; height: number }
  >();

  function walkElkNode(node: ElkNode) {
    if (
      node.id !== 'root' &&
      typeof node.x === 'number' &&
      typeof node.y === 'number'
    ) {
      elkCoords.set(node.id, {
        x: node.x,
        y: node.y,
        width: node.width ?? 360,
        height: node.height ?? 220,
      });
    }
    for (const child of node.children ?? []) {
      walkElkNode(child);
    }
  }
  walkElkNode(laidOutGraph);

  const rfNodes: RFNode[] = [];

  const orderedGroups = sortGroupsTopologically(activeGroups);
  for (const grp of orderedGroups) {
    const box = elkCoords.get(grp.id) ?? {
      x: 40,
      y: 40,
      width: 440,
      height: 260,
    };
    const parentId =
      grp.parent_group_id && activeGroupIDs.has(grp.parent_group_id)
        ? grp.parent_group_id
        : undefined;

    rfNodes.push({
      id: grp.id,
      type: 'cloudGroupNode',
      position: { x: box.x, y: box.y },
      parentId,
      style: {
        width: Math.max(box.width, 340),
        height: Math.max(box.height, 200),
      },
      data: { group: grp } satisfies CloudGroupNodeData,
      draggable: true,
      selectable: false,
    });
  }

  filtered.nodes.forEach((n, index) => {
    const elkBox = elkCoords.get(n.id) ?? {
      x: (index % 5) * 320,
      y: Math.floor(index / 5) * 160,
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    };

    const parentId =
      n.parent_group_id && activeGroupIDs.has(n.parent_group_id)
        ? n.parent_group_id
        : undefined;

    const finalPosition = hasManualCoordinates(n)
      ? { x: n.position.x, y: n.position.y }
      : { x: elkBox.x, y: elkBox.y };

    rfNodes.push({
      id: n.id,
      type: 'devopsNode',
      position: finalPosition,
      parentId,
      extent: parentId ? 'parent' : undefined,
      data: { node: n } satisfies DevOpsNodeData,
    });
  });

  const rfEdges: RFEdge[] = filtered.edges.map((e) => {
    const strokeColor = e.animated
      ? '#22d3ee'
      : RELATION_COLORS[e.relation_type] ?? '#64748b';

    return {
      id: e.id,
      source: e.source,
      target: e.target,
      type: 'smoothstep',
      pathOptions: { borderRadius: 14, offset: 22 },
      label: e.label || e.relation_type.replace(/_/g, ' '),
      labelStyle: {
        fill: '#e2e8f0',
        fontSize: 10,
        fontWeight: 600,
      },
      labelBgStyle: {
        fill: '#0f172a',
        fillOpacity: 0.92,
        stroke: '#1e293b',
        strokeWidth: 1,
      },
      labelBgPadding: [6, 3] as [number, number],
      labelBgBorderRadius: 6,
      animated: e.animated,
      style: {
        stroke: strokeColor,
        strokeWidth: e.animated ? 2.8 : e.is_manual ? 2.2 : 1.75,
        strokeDasharray: e.is_manual ? '6 4' : undefined,
      },
      markerEnd: {
        type: MarkerType.ArrowClosed,
        color: strokeColor,
        width: 18,
        height: 18,
      },
      data: { edge: e },
    };
  });

  return { nodes: rfNodes, edges: rfEdges };
}

function sortGroupsTopologically(groups: NodeGroup[]): NodeGroup[] {
  const byID = new Map(groups.map((g) => [g.id, g]));
  const visited = new Set<string>();
  const result: NodeGroup[] = [];

  function visit(g: NodeGroup) {
    if (visited.has(g.id)) return;
    visited.add(g.id);
    if (g.parent_group_id && byID.has(g.parent_group_id)) {
      visit(byID.get(g.parent_group_id)!);
    }
    result.push(g);
  }

  for (const g of groups) {
    visit(g);
  }
  return result;
}
