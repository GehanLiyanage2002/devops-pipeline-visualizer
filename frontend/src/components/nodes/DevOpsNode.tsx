import React, { memo, useMemo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import {
  AlertTriangle,
  Blocks,
  Box,
  CheckCircle2,
  Cloud,
  Container,
  Cpu,
  FileCode2,
  GitBranch,
  GitMerge,
  Loader2,
  ServerCog,
  Sparkles,
  Tag,
  Workflow,
  XCircle,
} from 'lucide-react';
import { usePipelineStore } from '../../store/usePipelineStore';
import {
  computeBlastRadius,
  type DevOpsNodeData,
} from '../../utils/layout';
import {
  extractNodeDisplayTags,
  extractNodeSubtitle,
} from '../../utils/gifExporter';
import type { NodeStatus, Stage, Tool } from '../../types/graph';

interface ToolTheme {
  label: string;
  Icon: React.ComponentType<{ className?: string }>;
  badgeBg: string;
  badgeText: string;
  accentBorder: string;
  iconBg: string;
}

const TOOL_THEMES: Record<Tool, ToolTheme> = {
  github: {
    label: 'GitHub',
    Icon: GitBranch,
    badgeBg: 'bg-slate-800/90 border-slate-600/60',
    badgeText: 'text-slate-200',
    accentBorder: 'border-l-slate-300',
    iconBg: 'bg-slate-800 text-white',
  },
  github_actions: {
    label: 'GitHub Actions',
    Icon: Workflow,
    badgeBg: 'bg-blue-950/80 border-blue-500/40',
    badgeText: 'text-blue-300',
    accentBorder: 'border-l-blue-400',
    iconBg: 'bg-blue-500/20 text-blue-400',
  },
  ansible: {
    label: 'Ansible',
    Icon: ServerCog,
    badgeBg: 'bg-rose-950/80 border-rose-500/40',
    badgeText: 'text-rose-300',
    accentBorder: 'border-l-rose-400',
    iconBg: 'bg-rose-500/20 text-rose-400',
  },
  docker: {
    label: 'Docker',
    Icon: Container,
    badgeBg: 'bg-sky-950/80 border-sky-500/40',
    badgeText: 'text-sky-300',
    accentBorder: 'border-l-sky-400',
    iconBg: 'bg-sky-500/20 text-sky-400',
  },
  terraform: {
    label: 'Terraform',
    Icon: Blocks,
    badgeBg: 'bg-purple-950/80 border-purple-500/40',
    badgeText: 'text-purple-300',
    accentBorder: 'border-l-purple-400',
    iconBg: 'bg-purple-500/20 text-purple-400',
  },
  argocd: {
    label: 'Argo CD',
    Icon: GitMerge,
    badgeBg: 'bg-orange-950/80 border-orange-500/40',
    badgeText: 'text-orange-300',
    accentBorder: 'border-l-orange-400',
    iconBg: 'bg-orange-500/20 text-orange-400',
  },
  kubernetes: {
    label: 'Kubernetes',
    Icon: Cpu,
    badgeBg: 'bg-indigo-950/80 border-indigo-500/40',
    badgeText: 'text-indigo-300',
    accentBorder: 'border-l-indigo-400',
    iconBg: 'bg-indigo-500/20 text-indigo-400',
  },
  aws: {
    label: 'AWS Cloud',
    Icon: Cloud,
    badgeBg: 'bg-amber-950/80 border-amber-500/40',
    badgeText: 'text-amber-300',
    accentBorder: 'border-l-amber-400',
    iconBg: 'bg-amber-500/20 text-amber-400',
  },
  azure: {
    label: 'Azure Cloud',
    Icon: Box,
    badgeBg: 'bg-cyan-950/80 border-cyan-500/40',
    badgeText: 'text-cyan-300',
    accentBorder: 'border-l-cyan-400',
    iconBg: 'bg-cyan-500/20 text-cyan-400',
  },
};

const STAGE_SHORT_LABELS: Record<Stage, string> = {
  source: 'SOURCE',
  ci: 'CI BUILD',
  config_mgmt: 'CONFIG MGMT',
  artifact: 'ARTIFACT',
  iac: 'TERRAFORM IAC',
  gitops: 'GITOPS SYNC',
  runtime: 'K8S RUNTIME',
  cloud: 'CLOUD INFRA',
};

function renderStatusBadge(status: NodeStatus) {
  switch (status) {
    case 'running':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-cyan-500/20 text-cyan-300 border border-cyan-400/50 animate-pulse">
          <Loader2 className="w-3 h-3 animate-spin" />
          RUNNING
        </span>
      );
    case 'success':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-500/20 text-emerald-300 border border-emerald-500/40">
          <CheckCircle2 className="w-3 h-3" />
          SUCCESS
        </span>
      );
    case 'failed':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-rose-500/20 text-rose-300 border border-rose-500/50">
          <XCircle className="w-3 h-3" />
          FAILED
        </span>
      );
    case 'out_of_sync':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-amber-500/20 text-amber-300 border border-amber-500/50">
          <AlertTriangle className="w-3 h-3" />
          OUT OF SYNC
        </span>
      );
    default:
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-800/90 text-slate-400 border border-slate-700/70">
          IDLE
        </span>
      );
  }
}

function getTagChipColorClass(tag: string): string {
  const lower = tag.toLowerCase();
  if (lower.startsWith('tag:') || lower.startsWith('img:') || lower.startsWith('repo:') || lower.startsWith('base:')) {
    return 'bg-sky-950/80 text-sky-300 border-sky-500/35';
  }
  if (lower.startsWith('ns:') || lower.startsWith('replicas:') || lower.startsWith('port:') || lower.startsWith('host:')) {
    return 'bg-indigo-950/80 text-indigo-300 border-indigo-500/35';
  }
  if (lower.startsWith('cidr:') || lower.startsWith('region:') || lower.startsWith('size:') || lower.startsWith('cloud:')) {
    return 'bg-amber-950/80 text-amber-300 border-amber-500/35';
  }
  if (lower.startsWith('branch:') || lower.startsWith('on:') || lower.startsWith('rev:') || lower.startsWith('path:')) {
    return 'bg-emerald-950/80 text-emerald-300 border-emerald-500/35';
  }
  if (lower.includes('=')) {
    return 'bg-purple-950/80 text-purple-300 border-purple-500/35';
  }
  return 'bg-slate-950/90 text-cyan-300/90 border-slate-700/80';
}

function getStatusGlowClass(
  status: NodeStatus,
  selected?: boolean,
  isSearchMatch?: boolean,
  isUpstream?: boolean,
  isDownstream?: boolean
): string {
  if (selected || isSearchMatch) {
    return 'ring-2 ring-cyan-400 border-cyan-400/90 shadow-glow-cyan';
  }
  if (isDownstream) {
    return 'ring-1 ring-amber-400/80 border-amber-400/80';
  }
  if (isUpstream) {
    return 'ring-1 ring-sky-400/80 border-sky-400/80';
  }
  switch (status) {
    case 'running':
      return 'border-cyan-400/90 shadow-glow-cyan';
    case 'success':
      return 'border-emerald-500/60 shadow-glow-emerald';
    case 'failed':
      return 'border-rose-500/80 shadow-glow-rose';
    case 'out_of_sync':
      return 'border-amber-500/80 shadow-glow-amber';
    default:
      return 'border-slate-800/90 hover:border-slate-600';
  }
}

const DevOpsNodeComponent: React.FC<NodeProps> = ({ data, selected }) => {
  const { node } = data as unknown as DevOpsNodeData;
  const searchQuery = usePipelineStore((s) => s.searchQuery);
  const activeToolFilter = usePipelineStore((s) => s.activeToolFilter);
  const showNodeTags = usePipelineStore((s) => s.showNodeTags);
  const highlightBlastRadius = usePipelineStore((s) => s.highlightBlastRadius);
  const selectedNodeId = usePipelineStore((s) => s.selectedNodeId);
  const rfEdges = usePipelineStore((s) => s.rfEdges);

  const theme = TOOL_THEMES[node.tool] ?? TOOL_THEMES.github;
  const Icon = theme.Icon;

  const subtitle = useMemo(() => extractNodeSubtitle(node), [node]);
  const tags = useMemo(() => extractNodeDisplayTags(node), [node]);

  const { isUpstream, isDownstream, isUnrelatedToSelection } = useMemo(() => {
    if (!highlightBlastRadius || !selectedNodeId || selectedNodeId === node.id) {
      return { isUpstream: false, isDownstream: false, isUnrelatedToSelection: false };
    }
    const { upstreamNodes, downstreamNodes } = computeBlastRadius(selectedNodeId, rfEdges);
    const up = upstreamNodes.has(node.id);
    const down = downstreamNodes.has(node.id);
    return {
      isUpstream: up,
      isDownstream: down,
      isUnrelatedToSelection: !up && !down,
    };
  }, [highlightBlastRadius, selectedNodeId, node.id, rfEdges]);

  const normalizedQuery = searchQuery.trim().toLowerCase();
  const isSearchMatch =
    normalizedQuery.length > 0 &&
    (node.label.toLowerCase().includes(normalizedQuery) ||
      node.id.toLowerCase().includes(normalizedQuery) ||
      node.tool.toLowerCase().includes(normalizedQuery) ||
      node.stage.toLowerCase().includes(normalizedQuery) ||
      subtitle.toLowerCase().includes(normalizedQuery) ||
      tags.some((t) => t.toLowerCase().includes(normalizedQuery)));

  const isToolFilteredOut =
    activeToolFilter !== 'all' && node.tool !== activeToolFilter;

  const isDimmed =
    (normalizedQuery.length > 0 && !isSearchMatch) ||
    isToolFilteredOut ||
    isUnrelatedToSelection;

  return (
    <div
      className={`w-[288px] rounded-xl bg-slate-900/95 backdrop-blur-md border border-l-4 ${
        theme.accentBorder
      } ${getStatusGlowClass(
        node.status,
        selected,
        isSearchMatch,
        isUpstream,
        isDownstream
      )} ${
        isDimmed ? 'opacity-25 scale-[0.98]' : 'opacity-100'
      } px-3.5 py-2.5 transition-all duration-200 select-none`}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!w-2.5 !h-2.5 !bg-slate-300 !border-2 !border-slate-900"
      />

      {/* Row 1: Tool Badge + Stage Badge + Live Status Badge */}
      <div className="flex items-center justify-between gap-1.5 mb-1.5">
        <div className="flex items-center gap-1.5 min-w-0">
          <div className={`p-1.5 rounded-lg ${theme.iconBg} shrink-0`}>
            <Icon className="w-3.5 h-3.5" />
          </div>
          <span
            className={`text-[10px] font-semibold uppercase tracking-wider px-1.5 py-0.5 rounded border ${theme.badgeBg} ${theme.badgeText} truncate`}
          >
            {theme.label}
          </span>
          <span className="text-[9px] font-mono uppercase px-1.5 py-0.5 rounded bg-slate-950/90 text-slate-400 border border-slate-800 shrink-0">
            {STAGE_SHORT_LABELS[node.stage] ?? node.stage}
          </span>
          {node.is_manual && (
            <span className="inline-flex items-center gap-0.5 text-[9px] font-semibold px-1.5 py-0.5 rounded bg-pink-500/20 text-pink-300 border border-pink-500/40 shrink-0">
              <Sparkles className="w-2.5 h-2.5" />
              MANUAL
            </span>
          )}
        </div>
        {renderStatusBadge(node.status)}
      </div>

      {/* Row 2: Primary Clear Node Name / Label + Upstream/Downstream Role Pill */}
      <div className="flex items-center justify-between gap-1.5 mb-1">
        <div
          className="text-[12.5px] font-bold text-slate-50 truncate"
          title={node.label}
        >
          {node.label}
        </div>
        {isUpstream && (
          <span className="text-[9px] font-mono uppercase px-1.5 py-0.5 rounded bg-sky-500/20 text-sky-300 border border-sky-400/40 shrink-0">
            UPSTREAM
          </span>
        )}
        {isDownstream && (
          <span className="text-[9px] font-mono uppercase px-1.5 py-0.5 rounded bg-amber-500/20 text-amber-300 border border-amber-400/40 shrink-0">
            IMPACT
          </span>
        )}
      </div>

      {/* Row 3: Technical Identifier & File Path Subtitle */}
      <div
        className="flex items-center gap-1 text-[10px] font-mono text-slate-400 truncate mb-2"
        title={subtitle}
      >
        <FileCode2 className="w-3 h-3 text-slate-500 shrink-0" />
        <span className="truncate">{subtitle}</span>
      </div>

      {/* Row 4: Categorized Semantic Tag Chips */}
      {showNodeTags && tags.length > 0 && (
        <div className="flex flex-wrap items-center gap-1 pt-1 border-t border-slate-800/80">
          {tags.slice(0, 3).map((tag, idx) => (
            <span
              key={idx}
              className={`inline-flex items-center gap-1 text-[9.5px] font-mono px-1.5 py-0.5 rounded border max-w-[175px] truncate ${getTagChipColorClass(
                tag
              )}`}
              title={tag}
            >
              <Tag className="w-2.5 h-2.5 opacity-75 shrink-0" />
              <span className="truncate">{tag}</span>
            </span>
          ))}
          {tags.length > 3 && (
            <span
              className="text-[9px] font-mono px-1.5 py-0.5 rounded bg-slate-800/80 text-slate-300 border border-slate-700"
              title={tags.slice(3).join(', ')}
            >
              +{tags.length - 3}
            </span>
          )}
        </div>
      )}

      <Handle
        type="source"
        position={Position.Right}
        className="!w-2.5 !h-2.5 !bg-cyan-400 !border-2 !border-slate-900"
      />
    </div>
  );
};

export const DevOpsNode = memo(DevOpsNodeComponent);
