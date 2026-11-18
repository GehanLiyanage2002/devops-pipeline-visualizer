import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Node as RFNode,
  type NodeTypes,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {
  Eye,
  Filter,
  GitFork,
  Info,
  Layers,
  LayoutGrid,
  Microscope,
  Radio,
  Search,
  Tag,
  X,
} from 'lucide-react';
import { DevOpsNode } from './nodes/DevOpsNode';
import { CloudGroupNode } from './nodes/CloudGroupNode';
import {
  DRAG_DATA_FORMAT,
  type ToolboxNodeTemplate,
} from './ToolboxSidebar';
import { usePipelineStore } from '../store/usePipelineStore';
import { RELATION_COLORS } from '../utils/layout';
import type { Node as PipelineNode, Tool, ZoomLevel } from '../types/graph';

const ZOOM_LEVEL_OPTIONS: {
  level: ZoomLevel;
  title: string;
  Icon: React.ComponentType<{ className?: string }>;
}[] = [
  {
    level: 1,
    title: 'Level 1: Executive Pipeline Flow',
    Icon: Layers,
  },
  {
    level: 2,
    title: 'Level 2: Jobs & Services',
    Icon: LayoutGrid,
  },
  {
    level: 3,
    title: 'Level 3: Full Granular Infrastructure',
    Icon: Microscope,
  },
];

const TOOL_FILTER_LABELS: Record<Tool, string> = {
  github: 'GitHub',
  github_actions: 'Actions',
  docker: 'Docker',
  terraform: 'Terraform',
  ansible: 'Ansible',
  argocd: 'Argo CD',
  kubernetes: 'Kubernetes',
  aws: 'AWS',
  azure: 'Azure',
};

const PipelineCanvasInner: React.FC = () => {
  const {
    graph,
    rfNodes,
    rfEdges,
    zoomLevel,
    layoutVersion,
    searchQuery,
    activeToolFilter,
    showEdgeLabels,
    showNodeTags,
    highlightBlastRadius,
    setSearchQuery,
    setActiveToolFilter,
    setShowEdgeLabels,
    setShowNodeTags,
    setHighlightBlastRadius,
    setZoomLevel,
    setSelectedNodeId,
    onNodesChange,
    onEdgesChange,
    onNodeDragStop,
    onConnectManualEdge,
    addCustomNode,
    wsConnected,
    telemetryMessage,
  } = usePipelineStore();

  const [showLegend, setShowLegend] = useState(true);
  const { screenToFlowPosition, fitView } = useReactFlow();

  useEffect(() => {
    if (layoutVersion === 0) return;
    const timer = window.setTimeout(() => {
      void fitView({ duration: 320, padding: 0.16 });
    }, 45);
    return () => window.clearTimeout(timer);
  }, [layoutVersion, fitView]);

  const nodeTypes: NodeTypes = useMemo(
    () => ({
      devopsNode: DevOpsNode,
      cloudGroupNode: CloudGroupNode,
    }),
    []
  );

  const presentTools = useMemo(() => {
    const set = new Set<Tool>();
    for (const n of graph?.nodes ?? []) {
      set.add(n.tool);
    }
    return Array.from(set);
  }, [graph]);

  const statusCounts = useMemo(() => {
    const counts = { running: 0, success: 0, failed: 0, idle: 0 };
    for (const n of graph?.nodes ?? []) {
      if (n.status === 'running') counts.running++;
      else if (n.status === 'success') counts.success++;
      else if (n.status === 'failed' || n.status === 'out_of_sync') counts.failed++;
      else counts.idle++;
    }
    return counts;
  }, [graph]);

  const handleDragOver = useCallback((event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  const handleDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      const raw = event.dataTransfer.getData(DRAG_DATA_FORMAT);
      if (!raw) return;

      try {
        const template: ToolboxNodeTemplate = JSON.parse(raw);
        const position = screenToFlowPosition({
          x: event.clientX,
          y: event.clientY,
        });
        const suffix = Math.floor(100 + Math.random() * 900);
        const newNode: PipelineNode = {
          id: `${template.idPrefix}-${suffix}`,
          label: template.defaultNodeLabel,
          stage: template.stage,
          tool: template.tool,
          status: 'idle',
          zoom_level: 2,
          position,
          is_manual: true,
          metadata: {
            ...template.defaultMetadata,
            correlation_reason: 'Manually dropped onto canvas from Architecture Toolbox',
          },
        };
        void addCustomNode(newNode);
      } catch {
        // Ignore invalid drag payload
      }
    },
    [screenToFlowPosition, addCustomNode]
  );

  return (
    <div
      className="relative w-full h-full bg-slate-950 overflow-hidden"
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      {/* Top-Left Node Search & Tool Filter Controls */}
      <div className="absolute top-4 left-4 z-20 flex flex-col gap-2 max-w-md">
        <div className="flex items-center gap-2 px-3 py-2 rounded-2xl bg-slate-900/90 border border-slate-800/90 shadow-xl backdrop-blur-md w-64">
          <Search className="w-3.5 h-3.5 text-slate-400 shrink-0" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Filter names, tags, images..."
            className="w-full bg-transparent text-xs text-slate-100 placeholder:text-slate-500 focus:outline-none"
          />
          {searchQuery && (
            <button
              type="button"
              onClick={() => setSearchQuery('')}
              className="text-slate-400 hover:text-white"
              aria-label="Clear search"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>

        {/* Tool Filter Pill Strip */}
        {presentTools.length > 0 && (
          <div className="flex flex-wrap items-center gap-1 p-1.5 rounded-xl bg-slate-900/90 border border-slate-800/90 shadow-lg backdrop-blur-md">
            <Filter className="w-3 h-3 text-slate-400 ml-1 mr-0.5 shrink-0" />
            <button
              type="button"
              onClick={() => setActiveToolFilter('all')}
              className={`px-2 py-0.5 rounded-lg text-[10px] font-semibold transition-colors ${
                activeToolFilter === 'all'
                  ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-400/50'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              All
            </button>
            {presentTools.map((tool) => (
              <button
                key={tool}
                type="button"
                onClick={() =>
                  setActiveToolFilter(activeToolFilter === tool ? 'all' : tool)
                }
                className={`px-2 py-0.5 rounded-lg text-[10px] font-medium transition-colors ${
                  activeToolFilter === tool
                    ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-400/50'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                {TOOL_FILTER_LABELS[tool] ?? tool}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Top Center Semantic Zoom Toggle Bar */}
      <div className="absolute top-4 left-1/2 -translate-x-1/2 z-20 flex items-center gap-1.5 p-1.5 rounded-2xl bg-slate-900/90 border border-slate-800/90 shadow-2xl backdrop-blur-md">
        {ZOOM_LEVEL_OPTIONS.map(({ level, title, Icon }) => {
          const active = zoomLevel === level;
          return (
            <button
              key={level}
              type="button"
              onClick={() => void setZoomLevel(level)}
              className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-medium transition-all ${
                active
                  ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-400/50 shadow-glow-cyan'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/70 border border-transparent'
              }`}
            >
              <Icon className="w-3.5 h-3.5 shrink-0" />
              <span>{title}</span>
            </button>
          );
        })}
      </div>

      {/* Top-Right Display & Label Toggles */}
      <div className="absolute top-4 right-4 z-20 flex items-center gap-1.5 p-1.5 rounded-2xl bg-slate-900/90 border border-slate-800/90 shadow-xl backdrop-blur-md">
        <button
          type="button"
          onClick={() => setShowNodeTags(!showNodeTags)}
          title="Toggle semantic node tags"
          className={`inline-flex items-center gap-1 px-2.5 py-1.5 rounded-xl text-[11px] font-medium transition-colors ${
            showNodeTags
              ? 'bg-cyan-500/15 text-cyan-300 border border-cyan-500/40'
              : 'text-slate-400 hover:text-slate-200 border border-transparent'
          }`}
        >
          <Tag className="w-3 h-3" />
          <span>Tags</span>
        </button>

        <button
          type="button"
          onClick={() => setShowEdgeLabels(!showEdgeLabels)}
          title="Toggle edge relation labels"
          className={`inline-flex items-center gap-1 px-2.5 py-1.5 rounded-xl text-[11px] font-medium transition-colors ${
            showEdgeLabels
              ? 'bg-cyan-500/15 text-cyan-300 border border-cyan-500/40'
              : 'text-slate-400 hover:text-slate-200 border border-transparent'
          }`}
        >
          <Eye className="w-3 h-3" />
          <span>Edge Labels</span>
        </button>

        <button
          type="button"
          onClick={() => setHighlightBlastRadius(!highlightBlastRadius)}
          title="Highlight upstream triggers & downstream blast radius when selecting a node"
          className={`inline-flex items-center gap-1 px-2.5 py-1.5 rounded-xl text-[11px] font-medium transition-colors ${
            highlightBlastRadius
              ? 'bg-amber-500/15 text-amber-300 border border-amber-500/40'
              : 'text-slate-400 hover:text-slate-200 border border-transparent'
          }`}
        >
          <GitFork className="w-3 h-3" />
          <span>Impact Chain</span>
        </button>

        <button
          type="button"
          onClick={() => setShowLegend(!showLegend)}
          title="Toggle relation color legend"
          className={`inline-flex items-center gap-1 px-2.5 py-1.5 rounded-xl text-[11px] font-medium transition-colors ${
            showLegend
              ? 'bg-slate-800 text-slate-200 border border-slate-700'
              : 'text-slate-400 hover:text-slate-200 border border-transparent'
          }`}
        >
          <Info className="w-3 h-3" />
          <span>Legend</span>
        </button>
      </div>

      {/* Relation Type Color Legend HUD */}
      {showLegend && (
        <div className="absolute bottom-14 left-16 z-20 flex flex-wrap items-center gap-3 px-3.5 py-2 rounded-xl bg-slate-900/90 border border-slate-800/90 text-[11px] text-slate-300 backdrop-blur-md shadow-lg">
          <span className="font-mono text-[10px] uppercase tracking-wider text-slate-400 font-semibold">
            Edge Relations:
          </span>
          {Object.entries(RELATION_COLORS).map(([rel, color]) => (
            <span key={rel} className="inline-flex items-center gap-1.5 font-mono text-[10.5px]">
              <span
                className="w-2.5 h-2.5 rounded-full shrink-0"
                style={{ backgroundColor: color }}
              />
              <span>{rel.replace(/_/g, ' ')}</span>
            </span>
          ))}
        </div>
      )}

      {/* Bottom Live WebSocket Telemetry & Status Counter Banner */}
      <div className="absolute bottom-4 left-16 z-20 flex items-center gap-3 px-3.5 py-1.5 rounded-xl bg-slate-900/90 border border-slate-800/90 text-xs text-slate-300 backdrop-blur-md shadow-lg">
        <span className="inline-flex items-center gap-1.5">
          <Radio
            className={`w-3.5 h-3.5 ${
              wsConnected ? 'text-emerald-400 animate-pulse' : 'text-slate-500'
            }`}
          />
          <span className="font-mono text-[11px] text-slate-400">
            {wsConnected ? 'LIVE TELEMETRY' : 'RECONNECTING...'}
          </span>
        </span>

        <span className="text-slate-700">|</span>

        <div className="flex items-center gap-2 font-mono text-[11px]">
          {statusCounts.running > 0 && (
            <span className="text-cyan-300">{statusCounts.running} running</span>
          )}
          <span className="text-emerald-400">{statusCounts.success} success</span>
          {statusCounts.failed > 0 && (
            <span className="text-rose-400">{statusCounts.failed} failed</span>
          )}
          <span className="text-slate-400">{statusCounts.idle} idle</span>
        </div>

        {telemetryMessage && (
          <>
            <span className="text-slate-700">|</span>
            <span className="text-cyan-300 font-medium">{telemetryMessage}</span>
          </>
        )}
      </div>

      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={(conn) => void onConnectManualEdge(conn)}
        onNodeClick={(_, node: RFNode) => {
          if (node.type === 'devopsNode') {
            setSelectedNodeId(node.id);
          }
        }}
        onPaneClick={() => setSelectedNodeId(null)}
        onNodeDragStop={(_, node: RFNode) => {
          if (node.type === 'devopsNode') {
            void onNodeDragStop(node.id, node.position);
          }
        }}
        fitView
        fitViewOptions={{ padding: 0.16 }}
        minZoom={0.18}
        maxZoom={1.8}
        className="bg-slate-950"
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={22}
          size={1.4}
          color="#334155"
        />
        <Controls position="bottom-left" />
        <MiniMap
          position="bottom-right"
          pannable
          zoomable
          nodeColor={(node) => {
            if (node.type === 'cloudGroupNode') return '#1e293b';
            return '#38bdf8';
          }}
          maskColor="rgba(2, 6, 23, 0.72)"
        />
      </ReactFlow>
    </div>
  );
};

export const PipelineCanvas: React.FC = () => (
  <ReactFlowProvider>
    <PipelineCanvasInner />
  </ReactFlowProvider>
);
