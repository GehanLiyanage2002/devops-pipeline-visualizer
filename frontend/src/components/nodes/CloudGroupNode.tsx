import React, { memo } from 'react';
import { type NodeProps } from '@xyflow/react';
import { Cloud, Cpu, Network, Shield } from 'lucide-react';
import type { CloudGroupNodeData } from '../../utils/layout';
import type { GroupProvider } from '../../types/graph';

const PROVIDER_STYLES: Record<
  GroupProvider,
  {
    border: string;
    bg: string;
    headerBg: string;
    headerText: string;
    badgeBg: string;
  }
> = {
  aws: {
    border: 'border-amber-500/45',
    bg: 'bg-amber-950/10',
    headerBg: 'bg-amber-950/80 border-b border-amber-500/30',
    headerText: 'text-amber-200',
    badgeBg: 'bg-amber-500/20 text-amber-300 border-amber-400/40',
  },
  azure: {
    border: 'border-sky-500/45',
    bg: 'bg-sky-950/10',
    headerBg: 'bg-sky-950/80 border-b border-sky-500/30',
    headerText: 'text-sky-200',
    badgeBg: 'bg-sky-500/20 text-sky-300 border-sky-400/40',
  },
  kubernetes: {
    border: 'border-indigo-500/45',
    bg: 'bg-indigo-950/10',
    headerBg: 'bg-indigo-950/80 border-b border-indigo-500/30',
    headerText: 'text-indigo-200',
    badgeBg: 'bg-indigo-500/20 text-indigo-300 border-indigo-400/40',
  },
};

const CloudGroupNodeComponent: React.FC<NodeProps> = ({ data }) => {
  const { group } = data as unknown as CloudGroupNodeData;
  const style = PROVIDER_STYLES[group.provider] ?? PROVIDER_STYLES.aws;

  const isCluster =
    group.resource_type === 'eks_cluster' ||
    group.resource_type === 'aks_cluster' ||
    group.resource_type === 'namespace';

  const Icon = isCluster ? Cpu : group.provider === 'aws' ? Cloud : Network;

  return (
    <div
      className={`w-full h-full rounded-2xl border-2 border-dashed ${style.border} ${style.bg} backdrop-blur-[2px] overflow-hidden pointer-events-none`}
    >
      <div
        className={`flex items-center justify-between px-3.5 py-2 ${style.headerBg}`}
      >
        <div className="flex items-center gap-2 min-w-0">
          <Icon className={`w-4 h-4 ${style.headerText} shrink-0`} />
          <span
            className={`text-xs font-semibold tracking-wide truncate ${style.headerText}`}
          >
            {group.label}
          </span>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <span
            className={`inline-flex items-center gap-1 text-[10px] font-mono uppercase px-2 py-0.5 rounded-full border ${style.badgeBg}`}
          >
            <Shield className="w-2.5 h-2.5" />
            {group.provider} • {group.resource_type.replace('_', ' ')}
          </span>
        </div>
      </div>
    </div>
  );
};

export const CloudGroupNode = memo(CloudGroupNodeComponent);
