import { create } from 'zustand';
import {
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type Edge as RFEdge,
  type EdgeChange,
  type Node as RFNode,
  type NodeChange,
} from '@xyflow/react';
import {
  computePipelineLayout,
  RELATION_COLORS,
  rollUpStatus,
  type DevOpsNodeData,
} from '../utils/layout';
import type {
  Edge as PipelineEdge,
  Node as PipelineNode,
  PipelineGraph,
  Position,
  TelemetryEvent,
  ZoomLevel,
} from '../types/graph';

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '';
const WS_URL =
  import.meta.env.VITE_WS_URL ??
  `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws/telemetry`;

export interface UploadedFilePayload {
  path: string;
  content: string;
}

interface PipelineStoreState {
  graph: PipelineGraph | null;
  rfNodes: RFNode[];
  rfEdges: RFEdge[];
  zoomLevel: ZoomLevel;
  layoutVersion: number;
  selectedNodeId: string | null;
  searchQuery: string;
  pendingEdgeLabel: string;
  isLoading: boolean;
  isSimulating: boolean;
  wsConnected: boolean;
  telemetryMessage: string | null;
  error: string | null;

  setGraph: (graph: PipelineGraph) => Promise<void>;
  setZoomLevel: (level: ZoomLevel) => Promise<void>;
  setSelectedNodeId: (id: string | null) => void;
  setSearchQuery: (query: string) => void;
  setPendingEdgeLabel: (label: string) => void;
  onNodesChange: (changes: NodeChange[]) => void;
  onEdgesChange: (changes: EdgeChange[]) => void;
  onNodeDragStop: (nodeId: string, position: Position) => Promise<void>;
  onConnectManualEdge: (connection: Connection) => Promise<void>;
  addCustomNode: (node: PipelineNode) => Promise<void>;
  updateNodeOverride: (node: PipelineNode) => Promise<void>;
  deleteManualNode: (nodeId: string) => Promise<void>;
  resetManualOverrides: () => Promise<void>;
  autoLayout: () => Promise<void>;
  loadSamplePipeline: () => Promise<void>;
  scanGitHubRepo: (repoUrl: string, branch: string, token?: string) => Promise<void>;
  parseUploadedFiles: (files: UploadedFilePayload[]) => Promise<void>;
  triggerSimulation: () => Promise<void>;
  connectWebSocket: () => () => void;
}

export const usePipelineStore = create<PipelineStoreState>((set, get) => ({
  graph: null,
  rfNodes: [],
  rfEdges: [],
  zoomLevel: 3,
  layoutVersion: 0,
  selectedNodeId: null,
  searchQuery: '',
  pendingEdgeLabel: 'manual link',
  isLoading: false,
  isSimulating: false,
  wsConnected: false,
  telemetryMessage: null,
  error: null,

  setGraph: async (graph: PipelineGraph) => {
    const { zoomLevel, layoutVersion } = get();
    const { nodes, edges } = await computePipelineLayout(graph, zoomLevel);
    set({
      graph,
      rfNodes: nodes,
      rfEdges: edges,
      layoutVersion: layoutVersion + 1,
      error: null,
    });
  },

  setZoomLevel: async (level: ZoomLevel) => {
    set({ zoomLevel: level });
    const { graph, layoutVersion } = get();
    if (graph) {
      const { nodes, edges } = await computePipelineLayout(graph, level);
      set({
        rfNodes: nodes,
        rfEdges: edges,
        layoutVersion: layoutVersion + 1,
      });
    }
  },

  setSelectedNodeId: (id: string | null) => {
    set({ selectedNodeId: id });
  },

  setSearchQuery: (query: string) => {
    set({ searchQuery: query });
  },

  setPendingEdgeLabel: (label: string) => {
    set({ pendingEdgeLabel: label });
  },

  onNodesChange: (changes: NodeChange[]) => {
    set({ rfNodes: applyNodeChanges(changes, get().rfNodes) });
  },

  onEdgesChange: (changes: EdgeChange[]) => {
    set({ rfEdges: applyEdgeChanges(changes, get().rfEdges) });
  },

  onNodeDragStop: async (nodeId: string, position: Position) => {
    const { graph } = get();
    if (!graph || nodeId.startsWith('stage-summary-') || nodeId.startsWith('tf-group-')) {
      return;
    }

    const updatedNodes = graph.nodes.map((n) =>
      n.id === nodeId ? { ...n, position } : n
    );
    set({ graph: { ...graph, nodes: updatedNodes } });

    try {
      await fetch(`${API_BASE}/api/pipeline/manual-override`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          node_positions: { [nodeId]: position },
        }),
      });
    } catch {
      // Keep optimistic position in state even if backend is offline
    }
  },

  onConnectManualEdge: async (connection: Connection) => {
    if (!connection.source || !connection.target) return;
    const currentGraph = get().graph;
    let sourceId = connection.source;
    let targetId = connection.target;

    // If connected while in Level 1 (stage-summary-*), resolve to the primary node in that stage
    if (sourceId.startsWith('stage-summary-') && currentGraph) {
      const stageName = sourceId.replace('stage-summary-', '');
      const match = currentGraph.nodes.find((n) => n.stage === stageName);
      if (match) sourceId = match.id;
    }
    if (targetId.startsWith('stage-summary-') && currentGraph) {
      const stageName = targetId.replace('stage-summary-', '');
      const match = currentGraph.nodes.find((n) => n.stage === stageName);
      if (match) targetId = match.id;
    }
    if (sourceId === targetId) return;

    const label = get().pendingEdgeLabel.trim() || 'manual link';
    const customEdge: PipelineEdge = {
      id: `${sourceId}->${targetId}:manual_link`,
      source: sourceId,
      target: targetId,
      relation_type: 'manual_link',
      animated: false,
      label,
      is_manual: true,
    };

    try {
      const res = await fetch(`${API_BASE}/api/pipeline/manual-override`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ custom_edges: [customEdge] }),
      });
      if (res.ok) {
        const updated: PipelineGraph = await res.json();
        await get().setGraph(updated);
      }
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to save manual link' });
    }
  },

  addCustomNode: async (node: PipelineNode) => {
    try {
      const res = await fetch(`${API_BASE}/api/pipeline/manual-override`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          custom_nodes: [{ ...node, is_manual: true }],
          node_positions: { [node.id]: node.position },
        }),
      });
      if (res.ok) {
        const updated: PipelineGraph = await res.json();
        await get().setGraph(updated);
        set({ selectedNodeId: node.id });
      }
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to add custom node' });
    }
  },

  updateNodeOverride: async (node: PipelineNode) => {
    try {
      const res = await fetch(`${API_BASE}/api/pipeline/manual-override`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          custom_nodes: [node],
        }),
      });
      if (res.ok) {
        const updated: PipelineGraph = await res.json();
        await get().setGraph(updated);
      }
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to update node' });
    }
  },

  deleteManualNode: async (nodeId: string) => {
    try {
      const res = await fetch(`${API_BASE}/api/pipeline/manual-override`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          delete_node_ids: [nodeId],
        }),
      });
      if (res.ok) {
        const updated: PipelineGraph = await res.json();
        set({ selectedNodeId: null });
        await get().setGraph(updated);
      }
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to delete manual node' });
    }
  },

  resetManualOverrides: async () => {
    try {
      const res = await fetch(`${API_BASE}/api/pipeline/manual-override`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reset: true }),
      });
      if (res.ok) {
        const updated: PipelineGraph = await res.json();
        await get().setGraph(updated);
      }
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to reset overrides' });
    }
  },

  autoLayout: async () => {
    const { graph, zoomLevel, layoutVersion } = get();
    if (!graph) return;

    try {
      const res = await fetch(`${API_BASE}/api/pipeline/manual-override`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ clear_positions: true }),
      });
      if (res.ok) {
        const updated: PipelineGraph = await res.json();
        await get().setGraph(updated);
        return;
      }
    } catch {
      // Fallback to local layout recomputation
    }

    const clearedGraph: PipelineGraph = {
      ...graph,
      nodes: graph.nodes.map((n) => ({
        ...n,
        position: { x: 0, y: 0 },
      })),
    };
    const { nodes, edges } = await computePipelineLayout(clearedGraph, zoomLevel);
    set({
      graph: clearedGraph,
      rfNodes: nodes,
      rfEdges: edges,
      layoutVersion: layoutVersion + 1,
    });
  },

  loadSamplePipeline: async () => {
    set({ isLoading: true, error: null });
    try {
      const res = await fetch(`${API_BASE}/api/pipeline/sample`);
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: ${await res.text()}`);
      }
      const data: PipelineGraph = await res.json();
      await get().setGraph(data);
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : 'Failed to load sample pipeline',
      });
    } finally {
      set({ isLoading: false });
    }
  },

  scanGitHubRepo: async (repoUrl: string, branch: string, token?: string) => {
    set({ isLoading: true, error: null });
    try {
      const res = await fetch(`${API_BASE}/api/pipeline/scan-github`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          repo_url: repoUrl,
          branch: branch || 'main',
          github_token: token ?? '',
        }),
      });
      if (!res.ok) {
        const bodyText = await res.text();
        throw new Error(`GitHub scan failed (${res.status}): ${bodyText}`);
      }
      const data: PipelineGraph = await res.json();
      await get().setGraph(data);
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : 'Failed to scan GitHub repository',
      });
      throw err;
    } finally {
      set({ isLoading: false });
    }
  },

  parseUploadedFiles: async (files: UploadedFilePayload[]) => {
    set({ isLoading: true, error: null });
    try {
      const res = await fetch(`${API_BASE}/api/pipeline/parse-files`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(files),
      });
      if (!res.ok) {
        const bodyText = await res.text();
        throw new Error(`File parse failed (${res.status}): ${bodyText}`);
      }
      const data: PipelineGraph = await res.json();
      await get().setGraph(data);
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : 'Failed to parse uploaded files',
      });
      throw err;
    } finally {
      set({ isLoading: false });
    }
  },

  triggerSimulation: async () => {
    set({
      isSimulating: true,
      telemetryMessage: 'Starting pipeline deployment simulation...',
    });
    try {
      await fetch(`${API_BASE}/api/pipeline/simulate-run`, { method: 'POST' });
    } catch (err) {
      set({
        isSimulating: false,
        error: err instanceof Error ? err.message : 'Failed to start simulation',
      });
    }
  },

  connectWebSocket: () => {
    let ws: WebSocket | null = null;
    let reconnectTimer: number | undefined;
    let disposed = false;

    const connect = () => {
      if (disposed) return;
      ws = new WebSocket(WS_URL);

      ws.onopen = () => {
        set({ wsConnected: true });
      };

      ws.onclose = () => {
        set({ wsConnected: false });
        if (!disposed) {
          reconnectTimer = window.setTimeout(connect, 2500);
        }
      };

      ws.onmessage = (msgEvt) => {
        try {
          const event: TelemetryEvent = JSON.parse(msgEvt.data);
          if (event.message) {
            set({ telemetryMessage: event.message });
          }
          if (event.type === 'simulation_completed') {
            set({ isSimulating: false });
          } else if (event.type === 'telemetry_update') {
            set({ isSimulating: true });
          }

          const currentGraph = get().graph;
          if (!currentGraph) return;

          const animatedSet = new Set(event.animated_edges ?? []);
          const updatedGraph: PipelineGraph = {
            ...currentGraph,
            nodes: currentGraph.nodes.map((n) => ({
              ...n,
              status: event.node_statuses?.[n.id] ?? n.status,
            })),
            edges: currentGraph.edges.map((e) => ({
              ...e,
              animated: animatedSet.has(e.id),
            })),
          };

          const updatedRFNodes = get().rfNodes.map((rfn) => {
            if (rfn.type !== 'devopsNode') return rfn;
            const nodeData = rfn.data as unknown as DevOpsNodeData;

            // Handle Level 1 Executive Stage Summary rollup during live simulation
            if (rfn.id.startsWith('stage-summary-')) {
              const memberIds =
                (nodeData.node.metadata?.member_ids as string[] | undefined) ?? [];
              const memberStatuses = memberIds.map(
                (id) => event.node_statuses?.[id] ?? 'idle'
              );
              const rolledStatus = rollUpStatus(memberStatuses);
              if (rolledStatus === nodeData.node.status) return rfn;
              return {
                ...rfn,
                data: {
                  ...nodeData,
                  node: { ...nodeData.node, status: rolledStatus },
                },
              };
            }

            const newStatus = event.node_statuses?.[rfn.id];
            if (!newStatus || newStatus === nodeData.node.status) return rfn;
            return {
              ...rfn,
              data: {
                ...nodeData,
                node: { ...nodeData.node, status: newStatus },
              },
            };
          });

          const runningStages = new Set(
            updatedRFNodes
              .filter(
                (rfn) =>
                  rfn.id.startsWith('stage-summary-') &&
                  (rfn.data as unknown as DevOpsNodeData).node.status === 'running'
              )
              .map((rfn) => rfn.id)
          );

          const updatedRFEdges = get().rfEdges.map((rfe) => {
            const isStageEdge =
              rfe.source.startsWith('stage-summary-') ||
              rfe.target.startsWith('stage-summary-');
            const isAnimated = isStageEdge
              ? runningStages.has(rfe.source) || runningStages.has(rfe.target)
              : animatedSet.has(rfe.id);

            const edgeObj = (rfe.data as { edge?: PipelineEdge } | undefined)?.edge;
            const defaultColor = edgeObj
              ? RELATION_COLORS[edgeObj.relation_type] ?? '#64748b'
              : '#64748b';

            return {
              ...rfe,
              animated: isAnimated,
              style: {
                ...rfe.style,
                stroke: isAnimated ? '#22d3ee' : defaultColor,
                strokeWidth: isAnimated ? 2.8 : 1.75,
              },
            };
          });

          set({
            graph: updatedGraph,
            rfNodes: updatedRFNodes,
            rfEdges: updatedRFEdges,
          });
        } catch {
          // Ignore malformed telemetry frame
        }
      };
    };

    connect();

    return () => {
      disposed = true;
      if (reconnectTimer) window.clearTimeout(reconnectTimer);
      if (ws) ws.close();
    };
  },
}));
