import React, { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import {
  AlertTriangle,
  Blocks,
  Box,
  CheckCircle2,
  Cloud,
  Container,
  Cpu,
  GitBranch,
  GitMerge,
  Loader2,
  ServerCog,
  Sparkles,
  Workflow,
  XCircle,
} from 'lucide-react';
import { usePipelineStore } from '../../store/usePipelineStore';
import type { DevOpsNodeData } from '../../utils/layout';
import type { NodeStatus, Tool } from '../../types/graph';

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

function getStatusGlowClass(
  status: NodeStatus,
  selected?: boolean,
  isSearchMatch?: boolean
): string {
  if (selected || isSearchMatch) {
    return 'ring-2 ring-cyan-400 border-cyan-400/90 shadow-glow-cyan';
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
  const theme = TOOL_THEMES[node.tool] ?? TOOL_THEMES.github;
  const Icon = theme.Icon;
  const meta = node.metadata ?? {};

  const normalizedQuery = searchQuery.trim().toLowerCase();
  const isSearchMatch =
    normalizedQuery.length > 0 &&
    (node.label.toLowerCase().includes(normalizedQuery) ||
      node.id.toLowerCase().includes(normalizedQuery) ||
      node.tool.toLowerCase().includes(normalizedQuery) ||
      node.stage.toLowerCase().includes(normalizedQuery) ||
      String(meta.container_image ?? '')
        .toLowerCase()
        .includes(normalizedQuery) ||
      String(meta.file_path ?? '')
        .toLowerCase()
        .includes(normalizedQuery));

  const isDimmed = normalizedQuery.length > 0 && !isSearchMatch;

  const pills: string[] = [];
  if (typeof meta.container_image === 'string' && meta.container_image) {
    pills.push(meta.container_image);
  } else if (typeof meta.image_repo === 'string' && meta.image_repo) {
    pills.push(meta.image_repo);
  }
  if (Array.isArray(meta.branches) && meta.branches.length > 0) {
    pills.push(`branch: ${String(meta.branches[0])}`);
  }
  if (typeof meta.runs_on === 'string' && meta.runs_on) {
    pills.push(meta.runs_on);
  }
  if (typeof meta.target_hosts === 'string' && meta.target_hosts) {
    pills.push(`hosts: ${meta.target_hosts}`);
  }
  if (typeof meta.resource_type === 'string' && meta.resource_type) {
    pills.push(meta.resource_type);
  }
  if (typeof meta.sync_path === 'string' && meta.sync_path) {
    pills.push(`path: ${meta.sync_path}`);
  }
  if (typeof meta.kind === 'string' && meta.kind) {
    pills.push(`k8s: ${meta.kind}`);
  }
  if (pills.length === 0 && typeof meta.file_path === 'string' && meta.file_path) {
    pills.push(meta.file_path);
  }

  return (
    <div
      className={`w-[272px] rounded-xl bg-slate-900/95 backdrop-blur-md border border-l-4 ${
        theme.accentBorder
      } ${getStatusGlowClass(node.status, selected, isSearchMatch)} ${
        isDimmed ? 'opacity-30 scale-[0.98]' : 'opacity-100'
      } px-3.5 py-2.5 transition-all duration-200 select-none`}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!w-2.5 !h-2.5 !bg-slate-300 !border-2 !border-slate-900"
      />

      {/* Top Row: Tool Badge + Live Status Badge */}
      <div className="flex items-center justify-between gap-1.5 mb-2">
        <div className="flex items-center gap-1.5 min-w-0">
          <div className={`p-1.5 rounded-lg ${theme.iconBg} shrink-0`}>
            <Icon className="w-3.5 h-3.5" />
          </div>
          <span
            className={`text-[10px] font-semibold uppercase tracking-wider px-1.5 py-0.5 rounded border ${theme.badgeBg} ${theme.badgeText} truncate`}
          >
            {theme.label}
          </span>
          {node.is_manual && (
            <span className="inline-flex items-center gap-0.5 text-[9px] font-semibold px-1.5 py-0.5 rounded bg-pink-500/20 text-pink-300 border border-pink-500/40">
              <Sparkles className="w-2.5 h-2.5" />
              MANUAL
            </span>
          )}
        </div>
        {renderStatusBadge(node.status)}
      </div>

      {/* Node Title */}
      <div
        className="text-xs font-semibold text-slate-100 truncate mb-2"
        title={node.label}
      >
        {node.label}
      </div>

      {/* Metadata Quick Pills */}
      <div className="flex flex-wrap items-center gap-1">
        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-slate-950/90 text-slate-400 border border-slate-800 uppercase">
          {node.stage}
        </span>
        {pills.slice(0, 2).map((pill, idx) => (
          <span
            key={idx}
            className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-slate-950/80 text-cyan-300/90 border border-slate-800 max-w-[180px] truncate"
            title={pill}
          >
            {pill}
          </span>
        ))}
      </div>

      <Handle
        type="source"
        position={Position.Right}
        className="!w-2.5 !h-2.5 !bg-cyan-400 !border-2 !border-slate-900"
      />
    </div>
  );
};

export const DevOpsNode = memo(DevOpsNodeComponent);
