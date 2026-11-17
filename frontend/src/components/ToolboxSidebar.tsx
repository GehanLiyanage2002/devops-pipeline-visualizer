import React from 'react';
import {
  Blocks,
  Box,
  Cloud,
  Container,
  Cpu,
  GitBranch,
  GitMerge,
  GripVertical,
  Link2,
  Plus,
  ServerCog,
  Workflow,
} from 'lucide-react';
import { usePipelineStore } from '../store/usePipelineStore';
import type { Node as PipelineNode, Stage, Tool } from '../types/graph';

export interface ToolboxNodeTemplate {
  idPrefix: string;
  label: string;
  defaultNodeLabel: string;
  tool: Tool;
  stage: Stage;
  description: string;
  colorClass: string;
  Icon: React.ComponentType<{ className?: string }>;
  defaultMetadata: Record<string, unknown>;
}

export const TOOLBOX_TEMPLATES: ToolboxNodeTemplate[] = [
  {
    idPrefix: 'manual-gh-repo',
    label: 'GitHub',
    defaultNodeLabel: 'GitHub Repository',
    tool: 'github',
    stage: 'source',
    description: 'Source code & webhook trigger',
    colorClass: 'text-slate-200 bg-slate-800 border-slate-700',
    Icon: GitBranch,
    defaultMetadata: { file_path: '.github/workflows/main.yml', branches: ['main'] },
  },
  {
    idPrefix: 'manual-gha-job',
    label: 'GitHub Actions',
    defaultNodeLabel: 'CI Workflow Job',
    tool: 'github_actions',
    stage: 'ci',
    description: 'CI build, test & release runner',
    colorClass: 'text-blue-400 bg-blue-500/15 border-blue-500/30',
    Icon: Workflow,
    defaultMetadata: { runs_on: 'ubuntu-latest', file_path: '.github/workflows/ci.yml' },
  },
  {
    idPrefix: 'manual-ansible-play',
    label: 'Ansible',
    defaultNodeLabel: 'Ansible Playbook',
    tool: 'ansible',
    stage: 'config_mgmt',
    description: 'Host provisioning & playbooks',
    colorClass: 'text-rose-400 bg-rose-500/15 border-rose-500/30',
    Icon: ServerCog,
    defaultMetadata: { target_hosts: 'app_servers', file_path: 'ansible/playbook.yml' },
  },
  {
    idPrefix: 'manual-docker-img',
    label: 'Docker',
    defaultNodeLabel: 'Docker Container Image',
    tool: 'docker',
    stage: 'artifact',
    description: 'OCI image artifact & registry tag',
    colorClass: 'text-sky-400 bg-sky-500/15 border-sky-500/30',
    Icon: Container,
    defaultMetadata: {
      image_repo: 'ghcr.io/pipescope/service',
      container_image: 'ghcr.io/pipescope/service:latest',
      file_path: 'Dockerfile',
    },
  },
  {
    idPrefix: 'manual-tf-mod',
    label: 'Terraform',
    defaultNodeLabel: 'Terraform IaC Resource',
    tool: 'terraform',
    stage: 'iac',
    description: 'HCL cloud infrastructure module',
    colorClass: 'text-purple-400 bg-purple-500/15 border-purple-500/30',
    Icon: Blocks,
    defaultMetadata: { resource_type: 'aws_rds_cluster', file_path: 'terraform/main.tf' },
  },
  {
    idPrefix: 'manual-argocd-app',
    label: 'Argo CD',
    defaultNodeLabel: 'Argo CD Application',
    tool: 'argocd',
    stage: 'gitops',
    description: 'GitOps manifest controller',
    colorClass: 'text-orange-400 bg-orange-500/15 border-orange-500/30',
    Icon: GitMerge,
    defaultMetadata: { sync_path: 'k8s/', target_revision: 'HEAD' },
  },
  {
    idPrefix: 'manual-k8s-deploy',
    label: 'K8s Deployment',
    defaultNodeLabel: 'Kubernetes Workload',
    tool: 'kubernetes',
    stage: 'runtime',
    description: 'Pod deployment & service runtime',
    colorClass: 'text-indigo-400 bg-indigo-500/15 border-indigo-500/30',
    Icon: Cpu,
    defaultMetadata: {
      kind: 'Deployment',
      namespace: 'production',
      container_image: 'ghcr.io/pipescope/service:latest',
    },
  },
  {
    idPrefix: 'manual-aws-eks',
    label: 'AWS EKS',
    defaultNodeLabel: 'AWS EKS / Cloud Service',
    tool: 'aws',
    stage: 'cloud',
    description: 'Managed AWS cluster or resource',
    colorClass: 'text-amber-400 bg-amber-500/15 border-amber-500/30',
    Icon: Cloud,
    defaultMetadata: { resource_type: 'eks_cluster', aws_region: 'us-east-1' },
  },
  {
    idPrefix: 'manual-azure-aks',
    label: 'Azure AKS',
    defaultNodeLabel: 'Azure AKS / Cloud Service',
    tool: 'azure',
    stage: 'cloud',
    description: 'Managed Azure Kubernetes resource',
    colorClass: 'text-cyan-400 bg-cyan-500/15 border-cyan-500/30',
    Icon: Box,
    defaultMetadata: { resource_type: 'aks_cluster', region: 'eastus' },
  },
];

export const DRAG_DATA_FORMAT = 'application/pipescope-node-template';

export const ToolboxSidebar: React.FC = () => {
  const { pendingEdgeLabel, setPendingEdgeLabel, addCustomNode, graph } =
    usePipelineStore();

  const handleDragStart = (
    event: React.DragEvent<HTMLDivElement>,
    template: ToolboxNodeTemplate
  ) => {
    event.dataTransfer.setData(DRAG_DATA_FORMAT, JSON.stringify(template));
    event.dataTransfer.effectAllowed = 'move';
  };

  const handleQuickAdd = (template: ToolboxNodeTemplate) => {
    const suffix = Math.floor(100 + Math.random() * 900);
    const count = graph?.nodes.length ?? 0;
    const newNode: PipelineNode = {
      id: `${template.idPrefix}-${suffix}`,
      label: template.defaultNodeLabel,
      stage: template.stage,
      tool: template.tool,
      status: 'idle',
      zoom_level: 2,
      position: {
        x: 180 + (count % 4) * 280,
        y: 140 + Math.floor(count / 4) * 140,
      },
      is_manual: true,
      metadata: {
        ...template.defaultMetadata,
        correlation_reason: 'Manually added via PipeScope Architecture Toolbox',
      },
    };
    void addCustomNode(newNode);
  };

  return (
    <aside className="w-64 shrink-0 h-full bg-slate-900/90 border-r border-slate-800/90 flex flex-col select-none z-20 backdrop-blur-md">
      {/* Sidebar Header */}
      <div className="px-4 py-3.5 border-b border-slate-800/90">
        <h2 className="text-xs font-bold uppercase tracking-wider text-slate-200">
          Architecture Toolbox
        </h2>
        <p className="text-[11px] text-slate-400 mt-0.5">
          Drag nodes onto the canvas or draw manual edges between handles.
        </p>
      </div>

      {/* Draggable Node Palette */}
      <div className="flex-1 overflow-y-auto px-3 py-3 space-y-2">
        {TOOLBOX_TEMPLATES.map((tpl) => {
          const Icon = tpl.Icon;
          return (
            <div
              key={tpl.idPrefix}
              draggable
              onDragStart={(e) => handleDragStart(e, tpl)}
              className="group flex items-center justify-between gap-2 p-2.5 rounded-xl bg-slate-950/80 hover:bg-slate-800/80 border border-slate-800/90 hover:border-cyan-500/40 cursor-grab active:cursor-grabbing transition-all"
            >
              <div className="flex items-center gap-2.5 min-w-0">
                <GripVertical className="w-3.5 h-3.5 text-slate-600 group-hover:text-slate-400 shrink-0" />
                <div className={`p-1.5 rounded-lg border ${tpl.colorClass} shrink-0`}>
                  <Icon className="w-4 h-4" />
                </div>
                <div className="min-w-0">
                  <div className="text-xs font-semibold text-slate-100 truncate">
                    {tpl.label}
                  </div>
                  <div className="text-[10px] text-slate-400 truncate">
                    {tpl.description}
                  </div>
                </div>
              </div>

              <button
                type="button"
                onClick={() => handleQuickAdd(tpl)}
                title={`Add ${tpl.label} node to canvas`}
                className="p-1 rounded-lg text-slate-400 hover:text-cyan-300 hover:bg-slate-700/60 transition-colors shrink-0"
              >
                <Plus className="w-3.5 h-3.5" />
              </button>
            </div>
          );
        })}
      </div>

      {/* Custom Manual Edge Label Configurator */}
      <div className="p-3.5 border-t border-slate-800/90 bg-slate-950/60 space-y-2">
        <div className="flex items-center gap-1.5 text-[11px] font-semibold text-slate-300">
          <Link2 className="w-3.5 h-3.5 text-pink-400" />
          <span>Manual Link Label (onConnect)</span>
        </div>
        <input
          type="text"
          value={pendingEdgeLabel}
          onChange={(e) => setPendingEdgeLabel(e.target.value)}
          placeholder="e.g., triggers, reads secrets..."
          className="w-full px-2.5 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-xs text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-cyan-400"
        />
        <p className="text-[10px] text-slate-500 leading-relaxed">
          Connect any two node handles on the canvas to create a persisted{' '}
          <span className="font-mono text-pink-300">manual_link</span> edge with this label.
        </p>
      </div>
    </aside>
  );
};
