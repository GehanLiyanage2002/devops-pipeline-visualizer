import React, { useEffect, useState } from 'react';
import {
  ArrowDownRight,
  ArrowUpLeft,
  Check,
  FileCode2,
  GitCompareArrows,
  Layers,
  Plus,
  Save,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react';
import { usePipelineStore } from '../store/usePipelineStore';
import type { DevOpsNodeData } from '../utils/layout';
import type { Edge as PipelineEdge, Node as PipelineNode } from '../types/graph';

export const NodeInspector: React.FC = () => {
  const {
    graph,
    rfNodes,
    rfEdges,
    selectedNodeId,
    setSelectedNodeId,
    updateNodeOverride,
    deleteManualNode,
  } = usePipelineStore();

  // Support selecting both standard graph nodes and Level 1 stage-summary nodes
  const selectedNode: PipelineNode | undefined =
    graph?.nodes.find((n) => n.id === selectedNodeId) ??
    (rfNodes.find((rfn) => rfn.id === selectedNodeId && rfn.type === 'devopsNode')
      ?.data as unknown as DevOpsNodeData | undefined)?.node;

  const isStageSummary = Boolean(selectedNode?.id.startsWith('stage-summary-'));

  const [labelInput, setLabelInput] = useState('');
  const [newMetaKey, setNewMetaKey] = useState('');
  const [newMetaVal, setNewMetaVal] = useState('');
  const [editableMeta, setEditableMeta] = useState<Record<string, unknown>>({});
  const [savedToast, setSavedToast] = useState(false);

  useEffect(() => {
    if (selectedNode) {
      setLabelInput(selectedNode.label);
      setEditableMeta({ ...(selectedNode.metadata ?? {}) });
      setNewMetaKey('');
      setNewMetaVal('');
      setSavedToast(false);
    }
  }, [selectedNode]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && selectedNodeId) {
        setSelectedNodeId(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [selectedNodeId, setSelectedNodeId]);

  if (!selectedNode || !graph) {
    return null;
  }

  const activeEdges: PipelineEdge[] = isStageSummary
    ? rfEdges.map((rfe) => ({
        id: rfe.id,
        source: rfe.source,
        target: rfe.target,
        relation_type:
          (rfe.data as { edge?: PipelineEdge } | undefined)?.edge?.relation_type ??
          'triggers',
        animated: Boolean(rfe.animated),
        label: String(rfe.label ?? 'triggers'),
        is_manual: false,
      }))
    : graph.edges;

  const upstreamEdges = activeEdges.filter((e) => e.target === selectedNode.id);
  const downstreamEdges = activeEdges.filter((e) => e.source === selectedNode.id);

  const nodeLabelByID = new Map<string, string>(
    graph.nodes.map((n) => [n.id, n.label])
  );
  for (const rfn of rfNodes) {
    if (rfn.type === 'devopsNode') {
      const nd = (rfn.data as unknown as DevOpsNodeData).node;
      nodeLabelByID.set(nd.id, nd.label);
    }
  }

  const correlationReason =
    typeof editableMeta.correlation_reason === 'string' &&
    editableMeta.correlation_reason
      ? editableMeta.correlation_reason
      : upstreamEdges.length > 0
      ? `Connected via ${upstreamEdges
          .map((e) => `${e.source} (${e.relation_type})`)
          .join(', ')}`
      : 'Root / entry pipeline resource discovered via static analysis';

  const handleSave = async () => {
    if (isStageSummary) return;
    const updatedNode: PipelineNode = {
      ...selectedNode,
      label: labelInput.trim() || selectedNode.label,
      metadata: editableMeta,
    };
    await updateNodeOverride(updatedNode);
    setSavedToast(true);
    window.setTimeout(() => setSavedToast(false), 1800);
  };

  const handleAddMetaProperty = () => {
    const k = newMetaKey.trim();
    if (!k) return;
    setEditableMeta((prev) => ({
      ...prev,
      [k]: newMetaVal,
    }));
    setNewMetaKey('');
    setNewMetaVal('');
  };

  return (
    <aside className="w-96 shrink-0 h-full bg-slate-900/95 border-l border-slate-800/90 flex flex-col z-20 backdrop-blur-md shadow-2xl">
      {/* Drawer Header */}
      <div className="px-4 py-3.5 border-b border-slate-800/90 flex items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-[10px] font-mono uppercase px-2 py-0.5 rounded bg-cyan-500/15 text-cyan-300 border border-cyan-500/30">
              {selectedNode.tool}
            </span>
            <span className="text-[10px] font-mono uppercase px-2 py-0.5 rounded bg-slate-800 text-slate-300 border border-slate-700">
              {selectedNode.stage}
            </span>
            {selectedNode.is_manual && (
              <span className="inline-flex items-center gap-0.5 text-[10px] font-semibold px-2 py-0.5 rounded bg-pink-500/20 text-pink-300 border border-pink-500/40">
                <Sparkles className="w-2.5 h-2.5" />
                MANUAL
              </span>
            )}
          </div>
          <h3
            className="text-sm font-bold text-white truncate mt-1"
            title={selectedNode.label}
          >
            {selectedNode.label}
          </h3>
          <p className="text-[11px] font-mono text-slate-400 truncate">
            id: {selectedNode.id}
          </p>
        </div>

        <button
          type="button"
          onClick={() => setSelectedNodeId(null)}
          className="p-1.5 rounded-xl text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
          aria-label="Close inspector"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {/* Scrollable Inspector Body */}
      <div className="flex-1 overflow-y-auto p-4 space-y-5">
        {/* Live Status & Parent Group */}
        <div className="grid grid-cols-2 gap-2.5">
          <div className="p-2.5 rounded-xl bg-slate-950/80 border border-slate-800/90">
            <div className="text-[10px] uppercase tracking-wider text-slate-400">
              Live Status
            </div>
            <div className="text-xs font-bold uppercase text-cyan-300 mt-1">
              {selectedNode.status}
            </div>
          </div>

          <div className="p-2.5 rounded-xl bg-slate-950/80 border border-slate-800/90">
            <div className="text-[10px] uppercase tracking-wider text-slate-400">
              Boundary Group
            </div>
            <div
              className="text-xs font-mono text-slate-200 truncate mt-1"
              title={selectedNode.parent_group_id || 'Global Scope'}
            >
              {selectedNode.parent_group_id || 'Global Scope'}
            </div>
          </div>
        </div>

        {/* Correlation Rule Explanation */}
        <div className="p-3 rounded-xl bg-cyan-950/30 border border-cyan-500/30 space-y-1.5">
          <div className="flex items-center gap-1.5 text-xs font-semibold text-cyan-300">
            <GitCompareArrows className="w-4 h-4 shrink-0" />
            <span>Correlation & Discovery Rule</span>
          </div>
          <p className="text-xs text-slate-200 leading-relaxed">
            {correlationReason}
          </p>
        </div>

        {/* Editable Node Label */}
        {!isStageSummary && (
          <div className="space-y-1.5">
            <label className="block text-[11px] font-semibold uppercase tracking-wider text-slate-400">
              Node Display Label
            </label>
            <input
              type="text"
              value={labelInput}
              onChange={(e) => setLabelInput(e.target.value)}
              className="w-full px-3 py-1.5 rounded-xl bg-slate-950 border border-slate-800 text-xs text-slate-100 focus:outline-none focus:border-cyan-400"
            />
          </div>
        )}

        {/* Upstream & Downstream Dependencies */}
        <div className="space-y-2.5">
          <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
            <Layers className="w-3.5 h-3.5 text-slate-400" />
            <span>Topology Dependencies</span>
          </div>

          <div className="space-y-1.5">
            <div className="text-[11px] font-medium text-slate-400 flex items-center gap-1">
              <ArrowUpLeft className="w-3.5 h-3.5 text-sky-400" />
              <span>Upstream ({upstreamEdges.length})</span>
            </div>
            {upstreamEdges.length === 0 ? (
              <div className="text-[11px] text-slate-500 italic pl-4">
                No incoming edges
              </div>
            ) : (
              upstreamEdges.map((edge) => (
                <button
                  key={edge.id}
                  type="button"
                  onClick={() => setSelectedNodeId(edge.source)}
                  className="w-full flex items-center justify-between gap-2 px-2.5 py-1.5 rounded-lg bg-slate-950/90 hover:bg-slate-800 border border-slate-800 text-left transition-colors"
                >
                  <span className="text-xs font-medium text-slate-200 truncate">
                    {nodeLabelByID.get(edge.source) ?? edge.source}
                  </span>
                  <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-sky-500/15 text-sky-300 border border-sky-500/30 shrink-0">
                    {edge.relation_type}
                  </span>
                </button>
              ))
            )}
          </div>

          <div className="space-y-1.5 pt-1">
            <div className="text-[11px] font-medium text-slate-400 flex items-center gap-1">
              <ArrowDownRight className="w-3.5 h-3.5 text-emerald-400" />
              <span>Downstream ({downstreamEdges.length})</span>
            </div>
            {downstreamEdges.length === 0 ? (
              <div className="text-[11px] text-slate-500 italic pl-4">
                No outgoing edges
              </div>
            ) : (
              downstreamEdges.map((edge) => (
                <button
                  key={edge.id}
                  type="button"
                  onClick={() => setSelectedNodeId(edge.target)}
                  className="w-full flex items-center justify-between gap-2 px-2.5 py-1.5 rounded-lg bg-slate-950/90 hover:bg-slate-800 border border-slate-800 text-left transition-colors"
                >
                  <span className="text-xs font-medium text-slate-200 truncate">
                    {nodeLabelByID.get(edge.target) ?? edge.target}
                  </span>
                  <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-300 border border-emerald-500/30 shrink-0">
                    {edge.relation_type}
                  </span>
                </button>
              ))
            )}
          </div>
        </div>

        {/* Extracted & Custom Metadata */}
        <div className="space-y-2.5">
          <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
            <FileCode2 className="w-3.5 h-3.5 text-slate-400" />
            <span>Extracted Metadata</span>
          </div>

          <div className="space-y-2">
            {Object.entries(editableMeta).map(([key, value]) => {
              const isPrimitive =
                typeof value === 'string' ||
                typeof value === 'number' ||
                typeof value === 'boolean';
              return (
                <div
                  key={key}
                  className="p-2.5 rounded-xl bg-slate-950/80 border border-slate-800/90 space-y-1"
                >
                  <div className="text-[10px] font-mono text-cyan-400">{key}</div>
                  {isPrimitive && !isStageSummary ? (
                    <input
                      type="text"
                      value={String(value)}
                      onChange={(e) =>
                        setEditableMeta((prev) => ({
                          ...prev,
                          [key]: e.target.value,
                        }))
                      }
                      className="w-full bg-slate-900/90 border border-slate-800 rounded-lg px-2 py-1 text-xs font-mono text-slate-100 focus:outline-none focus:border-cyan-400"
                    />
                  ) : (
                    <pre className="text-[11px] font-mono text-slate-300 whitespace-pre-wrap break-all">
                      {typeof value === 'string'
                        ? value
                        : JSON.stringify(value, null, 2)}
                    </pre>
                  )}
                </div>
              );
            })}
          </div>

          {!isStageSummary && (
            <div className="p-2.5 rounded-xl bg-slate-950/60 border border-slate-800/80 space-y-2">
              <div className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">
                Add Metadata Property
              </div>
              <div className="flex gap-1.5">
                <input
                  type="text"
                  value={newMetaKey}
                  onChange={(e) => setNewMetaKey(e.target.value)}
                  placeholder="key (e.g. iam_role)"
                  className="w-1/2 px-2 py-1 rounded-lg bg-slate-900 border border-slate-800 text-xs font-mono text-slate-100"
                />
                <input
                  type="text"
                  value={newMetaVal}
                  onChange={(e) => setNewMetaVal(e.target.value)}
                  placeholder="value"
                  className="w-1/2 px-2 py-1 rounded-lg bg-slate-900 border border-slate-800 text-xs font-mono text-slate-100"
                />
                <button
                  type="button"
                  onClick={handleAddMetaProperty}
                  className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-cyan-300 shrink-0"
                  title="Add metadata field"
                >
                  <Plus className="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Footer Actions: Save Overrides & Delete Manual Node */}
      {!isStageSummary && (
        <div className="p-3.5 border-t border-slate-800/90 bg-slate-950/80 flex items-center justify-between gap-2">
          <button
            type="button"
            onClick={() => void handleSave()}
            className="flex-1 inline-flex items-center justify-center gap-1.5 px-3 py-2 rounded-xl text-xs font-semibold bg-cyan-500 hover:bg-cyan-400 text-slate-950 transition-all"
          >
            {savedToast ? (
              <>
                <Check className="w-3.5 h-3.5" />
                <span>Saved!</span>
              </>
            ) : (
              <>
                <Save className="w-3.5 h-3.5" />
                <span>Save Node Changes</span>
              </>
            )}
          </button>

          {selectedNode.is_manual && (
            <button
              type="button"
              onClick={() => void deleteManualNode(selectedNode.id)}
              className="inline-flex items-center gap-1 px-3 py-2 rounded-xl text-xs font-semibold bg-rose-500/20 hover:bg-rose-500/30 text-rose-300 border border-rose-500/40 transition-colors"
              title="Delete manual node"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>Delete</span>
            </button>
          )}
        </div>
      )}
    </aside>
  );
};
