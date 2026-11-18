import type { Edge as RFEdge, Node as RFNode } from '@xyflow/react';
import type {
  Edge as PipelineEdge,
  Node as PipelineNode,
  NodeGroup,
  NodeStatus,
  PipelineGraph,
  RelationType,
  Tool,
} from '../types/graph';
import {
  RELATION_COLORS,
  type CloudGroupNodeData,
  type DevOpsNodeData,
} from './layout';

// ============================================================================
// Tool Color Palette & Display Helpers for Canvas / SVG / GIF Rendering
// ============================================================================

const TOOL_ACCENT_HEX: Record<Tool, { main: string; bg: string; label: string }> = {
  github: { main: '#e2e8f0', bg: '#1e293b', label: 'GITHUB' },
  github_actions: { main: '#60a5fa', bg: '#172554', label: 'GH ACTIONS' },
  ansible: { main: '#fb7185', bg: '#4c0519', label: 'ANSIBLE' },
  docker: { main: '#38bdf8', bg: '#082f49', label: 'DOCKER' },
  terraform: { main: '#c084fc', bg: '#3b0764', label: 'TERRAFORM' },
  argocd: { main: '#fb923c', bg: '#431407', label: 'ARGO CD' },
  kubernetes: { main: '#818cf8', bg: '#1e1b4b', label: 'KUBERNETES' },
  aws: { main: '#fbbf24', bg: '#451a03', label: 'AWS CLOUD' },
  azure: { main: '#22d3ee', bg: '#083344', label: 'AZURE CLOUD' },
};

const STATUS_COLORS: Record<NodeStatus, { stroke: string; badgeBg: string; text: string }> = {
  idle: { stroke: '#334155', badgeBg: '#1e293b', text: '#94a3b8' },
  running: { stroke: '#22d3ee', badgeBg: '#083344', text: '#67e8f9' },
  success: { stroke: '#10b981', badgeBg: '#064e3b', text: '#6ee7b7' },
  failed: { stroke: '#f43f5e', badgeBg: '#4c0519', text: '#fda4af' },
  out_of_sync: { stroke: '#f59e0b', badgeBg: '#451a03', text: '#fcd34d' },
};

export function extractNodeDisplayTags(node: PipelineNode): string[] {
  const meta = node.metadata ?? {};
  const result: string[] = [];
  const addTag = (t: string) => {
    const clean = t.trim();
    if (clean && !result.includes(clean)) {
      result.push(clean);
    }
  };

  if (Array.isArray(meta.custom_tags)) {
    for (const ct of meta.custom_tags) {
      if (typeof ct === 'string') addTag(ct);
    }
  }
  if (Array.isArray(meta.tags)) {
    for (const t of meta.tags) {
      if (typeof t === 'string') addTag(t);
    }
  }
  if (typeof meta.container_image === 'string' && meta.container_image) {
    addTag(`img:${meta.container_image}`);
  } else if (typeof meta.image_repo === 'string' && meta.image_repo) {
    addTag(`repo:${meta.image_repo}`);
  }
  if (typeof meta.namespace === 'string' && meta.namespace) {
    addTag(`ns:${meta.namespace}`);
  }
  if (typeof meta.runs_on === 'string' && meta.runs_on) {
    addTag(`runner:${meta.runs_on}`);
  }
  if (typeof meta.target_hosts === 'string' && meta.target_hosts) {
    addTag(`hosts:${meta.target_hosts}`);
  }
  if (typeof meta.sync_path === 'string' && meta.sync_path) {
    addTag(`path:${meta.sync_path}`);
  }
  if (typeof meta.cidr_block === 'string' && meta.cidr_block) {
    addTag(`cidr:${meta.cidr_block}`);
  }
  if (typeof meta.instance_type === 'string' && meta.instance_type) {
    addTag(`size:${meta.instance_type}`);
  }

  return result.slice(0, 5);
}

export function extractNodeSubtitle(node: PipelineNode): string {
  const meta = node.metadata ?? {};
  if (typeof meta.tf_address === 'string' && meta.tf_address) {
    return `${meta.tf_address} • ${String(meta.file_path ?? '')}`;
  }
  if (typeof meta.kind === 'string' && meta.kind) {
    const ns = typeof meta.namespace === 'string' ? meta.namespace : 'default';
    return `${meta.kind} (${ns}) • ${String(meta.file_path ?? '')}`;
  }
  if (typeof meta.job_id === 'string' && meta.job_id) {
    return `job: ${meta.job_id} • ${String(meta.file_path ?? '')}`;
  }
  if (typeof meta.file_path === 'string' && meta.file_path) {
    return `${node.id} • ${meta.file_path}`;
  }
  return node.id;
}

// ============================================================================
// Absolute Coordinate Resolution for Compound React Flow Nodes
// ============================================================================

interface PositionedBox {
  id: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

function resolveAbsoluteNodeBoxes(rfNodes: RFNode[]): {
  nodeBoxes: Map<string, PositionedBox>;
  groupBoxes: Map<string, PositionedBox & { group: NodeGroup }>;
  bounds: { minX: number; minY: number; maxX: number; maxY: number };
} {
  const byId = new Map<string, RFNode>(rfNodes.map((n) => [n.id, n]));

  const getAbsPos = (n: RFNode): { x: number; y: number } => {
    let x = n.position?.x ?? 0;
    let y = n.position?.y ?? 0;
    let currParent = n.parentId;
    const seen = new Set<string>();
    while (currParent && !seen.has(currParent)) {
      seen.add(currParent);
      const pNode = byId.get(currParent);
      if (!pNode) break;
      x += pNode.position?.x ?? 0;
      y += pNode.position?.y ?? 0;
      currParent = pNode.parentId;
    }
    return { x, y };
  };

  const nodeBoxes = new Map<string, PositionedBox>();
  const groupBoxes = new Map<string, PositionedBox & { group: NodeGroup }>();

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;

  for (const rfn of rfNodes) {
    const abs = getAbsPos(rfn);
    const w =
      typeof rfn.style?.width === 'number'
        ? rfn.style.width
        : rfn.type === 'cloudGroupNode'
        ? 420
        : 288;
    const h =
      typeof rfn.style?.height === 'number'
        ? rfn.style.height
        : rfn.type === 'cloudGroupNode'
        ? 240
        : 138;

    minX = Math.min(minX, abs.x);
    minY = Math.min(minY, abs.y);
    maxX = Math.max(maxX, abs.x + w);
    maxY = Math.max(maxY, abs.y + h);

    if (rfn.type === 'cloudGroupNode') {
      const grp = (rfn.data as unknown as CloudGroupNodeData).group;
      groupBoxes.set(rfn.id, { id: rfn.id, x: abs.x, y: abs.y, width: w, height: h, group: grp });
    } else if (rfn.type === 'devopsNode') {
      nodeBoxes.set(rfn.id, { id: rfn.id, x: abs.x, y: abs.y, width: 288, height: 138 });
    }
  }

  if (!Number.isFinite(minX)) {
    minX = 0;
    minY = 0;
    maxX = 1200;
    maxY = 700;
  }

  return {
    nodeBoxes,
    groupBoxes,
    bounds: { minX, minY, maxX, maxY },
  };
}

// ============================================================================
// Shared Canvas Frame Renderer (Used by both High-Res PNG & Animated GIF)
// ============================================================================

interface RenderFrameOptions {
  width: number;
  height: number;
  graph: PipelineGraph;
  rfNodes: RFNode[];
  rfEdges: RFEdge[];
  statusOverrides?: Record<string, NodeStatus>;
  activeEdgeIds?: Set<string>;
  particleProgress?: number; // 0..1 along active edges
  phaseBanner?: string;
  phaseIndex?: number; // 0..6
}

function renderPipelineFrameToCanvas(
  ctx: CanvasRenderingContext2D,
  opts: RenderFrameOptions
): void {
  const {
    width,
    height,
    graph,
    rfNodes,
    rfEdges,
    statusOverrides,
    activeEdgeIds,
    particleProgress = 0,
    phaseBanner,
    phaseIndex = 0,
  } = opts;

  // 1. Background & Subtle Grid
  ctx.fillStyle = '#020617';
  ctx.fillRect(0, 0, width, height);

  ctx.fillStyle = '#1e293b';
  for (let gx = 28; gx < width; gx += 28) {
    for (let gy = 88; gy < height - 52; gy += 28) {
      ctx.fillRect(gx, gy, 1.5, 1.5);
    }
  }

  // 2. Top Header Banner
  ctx.fillStyle = '#0f172a';
  ctx.fillRect(0, 0, width, 76);
  ctx.strokeStyle = '#1e293b';
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  ctx.moveTo(0, 76);
  ctx.lineTo(width, 76);
  ctx.stroke();

  // Brand Title
  ctx.fillStyle = '#22d3ee';
  ctx.font = 'bold 20px Inter, system-ui, sans-serif';
  ctx.fillText(`PipeScope — ${graph.name}`, 28, 32);

  // Subtitle stats
  ctx.fillStyle = '#94a3b8';
  ctx.font = '12px monospace';
  ctx.fillText(
    `Nodes: ${graph.nodes.length}  •  Edges: ${graph.edges.length}  •  Cloud Groups: ${graph.groups.length}  •  Exported: ${new Date().toISOString().slice(0, 19)}Z`,
    28,
    54
  );

  // Live Stage Progress Pill in Header
  if (phaseBanner) {
    const pillW = 390;
    const pillX = width - pillW - 28;
    ctx.fillStyle = '#083344';
    ctx.strokeStyle = '#22d3ee';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.roundRect(pillX, 16, pillW, 44, 10);
    ctx.fill();
    ctx.stroke();

    ctx.fillStyle = '#67e8f9';
    ctx.font = 'bold 12px monospace';
    ctx.fillText(phaseBanner, pillX + 16, 35);

    // Progress bar inside pill
    const barW = pillW - 32;
    ctx.fillStyle = '#164e63';
    ctx.fillRect(pillX + 16, 44, barW, 6);
    ctx.fillStyle = '#22d3ee';
    ctx.fillRect(pillX + 16, 44, Math.min(barW, Math.max(24, (barW * phaseIndex) / 6)), 6);
  }

  // 3. Compute Viewport Scale & Offset to fit entire DAG cleanly
  const { nodeBoxes, groupBoxes, bounds } = resolveAbsoluteNodeBoxes(rfNodes);
  const padX = 56;
  const padTop = 104;
  const padBottom = 72;
  const availW = width - padX * 2;
  const availH = height - padTop - padBottom;
  const graphW = Math.max(bounds.maxX - bounds.minX, 400);
  const graphH = Math.max(bounds.maxY - bounds.minY, 260);
  const scale = Math.min(availW / graphW, availH / graphH, 1.05);

  const offsetX = padX + (availW - graphW * scale) / 2 - bounds.minX * scale;
  const offsetY = padTop + (availH - graphH * scale) / 2 - bounds.minY * scale;

  ctx.save();

  // 4. Draw Cloud Boundary Groups
  for (const [, gBox] of groupBoxes) {
    const gx = offsetX + gBox.x * scale;
    const gy = offsetY + gBox.y * scale;
    const gw = gBox.width * scale;
    const gh = gBox.height * scale;

    const isAWS = gBox.group.provider === 'aws';
    const borderCol = isAWS ? '#f59e0b' : gBox.group.provider === 'azure' ? '#38bdf8' : '#818cf8';
    const headerFill = isAWS ? 'rgba(69, 26, 3, 0.85)' : 'rgba(30, 27, 75, 0.85)';

    ctx.fillStyle = isAWS ? 'rgba(69, 26, 3, 0.14)' : 'rgba(30, 27, 75, 0.14)';
    ctx.strokeStyle = borderCol;
    ctx.lineWidth = 1.8;
    ctx.setLineDash([7, 5]);
    ctx.beginPath();
    ctx.roundRect(gx, gy, gw, gh, 14 * scale);
    ctx.fill();
    ctx.stroke();
    ctx.setLineDash([]);

    // Group Header Bar
    const headerH = Math.max(24, 30 * scale);
    ctx.fillStyle = headerFill;
    ctx.beginPath();
    ctx.roundRect(gx, gy, gw, headerH, [14 * scale, 14 * scale, 0, 0]);
    ctx.fill();

    ctx.fillStyle = isAWS ? '#fde68a' : '#c7d2fe';
    ctx.font = `bold ${Math.max(10, Math.round(12 * scale))}px Inter, sans-serif`;
    ctx.fillText(
      `${gBox.group.label}  [${gBox.group.provider.toUpperCase()} • ${gBox.group.resource_type.toUpperCase()}]`,
      gx + 12 * scale,
      gy + headerH * 0.68
    );
  }

  // 5. Draw Edges & Animated Data Flow Particles
  for (const rfe of rfEdges) {
    const srcBox = nodeBoxes.get(rfe.source);
    const tgtBox = nodeBoxes.get(rfe.target);
    if (!srcBox || !tgtBox) continue;

    const sx = offsetX + (srcBox.x + srcBox.width) * scale;
    const sy = offsetY + (srcBox.y + srcBox.height / 2) * scale;
    const tx = offsetX + tgtBox.x * scale;
    const ty = offsetY + (tgtBox.y + tgtBox.height / 2) * scale;

    const edgeData = (rfe.data as { edge?: PipelineEdge } | undefined)?.edge;
    const relType: RelationType = edgeData?.relation_type ?? 'triggers';
    const isEdgeActive = activeEdgeIds ? activeEdgeIds.has(rfe.id) : Boolean(rfe.animated);
    const strokeColor = isEdgeActive ? '#22d3ee' : RELATION_COLORS[relType] ?? '#64748b';

    const midX = (sx + tx) / 2;

    ctx.strokeStyle = strokeColor;
    ctx.lineWidth = isEdgeActive ? 2.8 : 1.8;
    ctx.beginPath();
    ctx.moveTo(sx, sy);
    ctx.bezierCurveTo(midX, sy, midX, ty, tx, ty);
    ctx.stroke();

    // Arrowhead at target
    const arrowSize = Math.max(6, 8 * scale);
    ctx.fillStyle = strokeColor;
    ctx.beginPath();
    ctx.moveTo(tx, ty);
    ctx.lineTo(tx - arrowSize, ty - arrowSize * 0.6);
    ctx.lineTo(tx - arrowSize, ty + arrowSize * 0.6);
    ctx.closePath();
    ctx.fill();

    // Animated glowing particles along active edges
    if (isEdgeActive) {
      for (const offset of [0, 0.5]) {
        const t = (particleProgress + offset) % 1;
        const inv = 1 - t;
        const px =
          inv * inv * inv * sx +
          3 * inv * inv * t * midX +
          3 * inv * t * t * midX +
          t * t * t * tx;
        const py =
          inv * inv * inv * sy +
          3 * inv * inv * t * sy +
          3 * inv * t * t * ty +
          t * t * t * ty;

        ctx.fillStyle = '#67e8f9';
        ctx.beginPath();
        ctx.arc(px, py, Math.max(4, 5.5 * scale), 0, Math.PI * 2);
        ctx.fill();
      }
    }

    // Edge Relation Label Pill
    const edgeLabel = String(rfe.label ?? edgeData?.label ?? relType.replace(/_/g, ' '));
    if (edgeLabel) {
      const lx = (sx + tx) / 2;
      const ly = (sy + ty) / 2;
      ctx.font = `bold ${Math.max(8, Math.round(9.5 * scale))}px monospace`;
      const textW = ctx.measureText(edgeLabel).width;
      const padH = 6 * scale;
      const pillH = Math.max(14, 16 * scale);

      ctx.fillStyle = '#0f172a';
      ctx.strokeStyle = strokeColor;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.roundRect(lx - textW / 2 - padH, ly - pillH / 2, textW + padH * 2, pillH, 4);
      ctx.fill();
      ctx.stroke();

      ctx.fillStyle = isEdgeActive ? '#67e8f9' : '#e2e8f0';
      ctx.fillText(edgeLabel, lx - textW / 2, ly + pillH * 0.26);
    }
  }

  // 6. Draw DevOps Nodes with Clear Labels, Technical Names & Tags
  for (const rfn of rfNodes) {
    if (rfn.type !== 'devopsNode') continue;
    const nBox = nodeBoxes.get(rfn.id);
    if (!nBox) continue;

    const node = (rfn.data as unknown as DevOpsNodeData).node;
    const status: NodeStatus = statusOverrides?.[node.id] ?? node.status;
    const toolTheme = TOOL_ACCENT_HEX[node.tool] ?? TOOL_ACCENT_HEX.github;
    const statusStyle = STATUS_COLORS[status] ?? STATUS_COLORS.idle;

    const nx = offsetX + nBox.x * scale;
    const ny = offsetY + nBox.y * scale;
    const nw = nBox.width * scale;
    const nh = nBox.height * scale;

    // Card Background & Status Border
    ctx.fillStyle = '#0f172a';
    ctx.strokeStyle = status === 'idle' ? toolTheme.main : statusStyle.stroke;
    ctx.lineWidth = status === 'running' ? 2.8 : 1.8;
    ctx.beginPath();
    ctx.roundRect(nx, ny, nw, nh, 10 * scale);
    ctx.fill();
    ctx.stroke();

    // Left Tool Accent Bar
    ctx.fillStyle = toolTheme.main;
    ctx.beginPath();
    ctx.roundRect(nx, ny, Math.max(4, 5 * scale), nh, [10 * scale, 0, 0, 10 * scale]);
    ctx.fill();

    // Row 1: Tool Badge + Stage Badge + Status Badge
    const padLeft = nx + 12 * scale;
    const topY = ny + 18 * scale;

    ctx.fillStyle = toolTheme.main;
    ctx.font = `bold ${Math.max(8, Math.round(10 * scale))}px monospace`;
    const toolLabelText = `${toolTheme.label} • ${node.stage.toUpperCase()}`;
    ctx.fillText(toolLabelText.slice(0, 26), padLeft, topY);

    // Status pill on top-right
    const statusText = status.toUpperCase();
    ctx.font = `bold ${Math.max(8, Math.round(9 * scale))}px monospace`;
    const stW = ctx.measureText(statusText).width + 10 * scale;
    const stX = nx + nw - stW - 8 * scale;
    ctx.fillStyle = statusStyle.badgeBg;
    ctx.beginPath();
    ctx.roundRect(stX, ny + 7 * scale, stW, 15 * scale, 6);
    ctx.fill();
    ctx.fillStyle = statusStyle.text;
    ctx.fillText(statusText, stX + 5 * scale, topY);

    // Row 2: Primary Clear Node Label
    ctx.fillStyle = '#f8fafc';
    ctx.font = `bold ${Math.max(10, Math.round(12.5 * scale))}px Inter, system-ui, sans-serif`;
    ctx.fillText(node.label.slice(0, 32), padLeft, ny + 42 * scale);

    // Row 3: Technical Subtitle (ID / Resource Address / File Path)
    const subtitle = extractNodeSubtitle(node);
    ctx.fillStyle = '#94a3b8';
    ctx.font = `${Math.max(8, Math.round(9.5 * scale))}px monospace`;
    ctx.fillText(subtitle.slice(0, 38), padLeft, ny + 60 * scale);

    // Row 4 & 5: Semantic Tags Pills
    const tags = extractNodeDisplayTags(node);
    let curTagX = padLeft;
    let curTagY = ny + 76 * scale;
    const maxTagRight = nx + nw - 10 * scale;

    ctx.font = `${Math.max(7.5, Math.round(9 * scale))}px monospace`;
    for (const tag of tags.slice(0, 3)) {
      const displayTag = tag.length > 26 ? tag.slice(0, 25) + '…' : tag;
      const tw = ctx.measureText(displayTag).width + 10 * scale;
      if (curTagX + tw > maxTagRight) {
        curTagX = padLeft;
        curTagY += 20 * scale;
      }
      if (curTagY + 14 * scale > ny + nh - 4 * scale) break;

      ctx.fillStyle = '#020617';
      ctx.strokeStyle = '#1e293b';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.roundRect(curTagX, curTagY, tw, 15 * scale, 4);
      ctx.fill();
      ctx.stroke();

      ctx.fillStyle = '#67e8f9';
      ctx.fillText(displayTag, curTagX + 5 * scale, curTagY + 11 * scale);
      curTagX += tw + 5 * scale;
    }
  }

  ctx.restore();

  // 7. Bottom Legend Footer Bar
  const footerY = height - 42;
  ctx.fillStyle = '#0f172a';
  ctx.fillRect(0, footerY, width, 42);
  ctx.strokeStyle = '#1e293b';
  ctx.beginPath();
  ctx.moveTo(0, footerY);
  ctx.lineTo(width, footerY);
  ctx.stroke();

  let legX = 28;
  ctx.fillStyle = '#94a3b8';
  ctx.font = 'bold 11px monospace';
  ctx.fillText('RELATIONS:', legX, footerY + 25);
  legX += 86;

  for (const [rel, color] of Object.entries(RELATION_COLORS)) {
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(legX, footerY + 21, 5, 0, Math.PI * 2);
    ctx.fill();

    ctx.fillStyle = '#cbd5e1';
    ctx.font = '11px monospace';
    const label = rel.replace(/_/g, ' ');
    ctx.fillText(label, legX + 10, footerY + 25);
    legX += ctx.measureText(label).width + 30;
  }
}

// ============================================================================
// 1. High-Resolution PNG Architecture Exporter
// ============================================================================

export function exportPipelinePNG(
  graph: PipelineGraph,
  rfNodes: RFNode[],
  rfEdges: RFEdge[]
): void {
  const canvas = document.createElement('canvas');
  canvas.width = 1800;
  canvas.height = 1040;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  renderPipelineFrameToCanvas(ctx, {
    width: canvas.width,
    height: canvas.height,
    graph,
    rfNodes,
    rfEdges,
    phaseBanner: 'STATIC ARCHITECTURE TOPOLOGY SNAPSHOT',
    phaseIndex: 6,
  });

  const dataUrl = canvas.toDataURL('image/png');
  const a = document.createElement('a');
  a.href = dataUrl;
  a.download = `${graph.id || 'pipescope-pipeline'}-architecture.png`;
  a.click();
}

// ============================================================================
// 2. Scalable Vector Graphics (SVG) Architecture Exporter
// ============================================================================

export function exportPipelineSVG(
  graph: PipelineGraph,
  rfNodes: RFNode[],
  rfEdges: RFEdge[]
): void {
  const { nodeBoxes, groupBoxes, bounds } = resolveAbsoluteNodeBoxes(rfNodes);
  const pad = 64;
  const vbX = Math.floor(bounds.minX - pad);
  const vbY = Math.floor(bounds.minY - pad - 40);
  const vbW = Math.ceil(bounds.maxX - bounds.minX + pad * 2);
  const vbH = Math.ceil(bounds.maxY - bounds.minY + pad * 2 + 60);

  const esc = (s: string) =>
    s
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');

  const lines: string[] = [];
  lines.push(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${vbX} ${vbY} ${vbW} ${vbH}" width="${Math.max(
      vbW,
      1400
    )}" height="${Math.max(vbH, 860)}" style="background:#020617;font-family:Inter,system-ui,sans-serif">`
  );

  // Title Header
  lines.push(
    `<text x="${vbX + 24}" y="${vbY + 34}" fill="#22d3ee" font-size="20" font-weight="bold">PipeScope — ${esc(
      graph.name
    )} (${graph.nodes.length} nodes, ${graph.edges.length} edges)</text>`
  );

  // Cloud Groups
  for (const [, gBox] of groupBoxes) {
    const isAWS = gBox.group.provider === 'aws';
    const stroke = isAWS ? '#f59e0b' : '#818cf8';
    lines.push(
      `<rect x="${gBox.x}" y="${gBox.y}" width="${gBox.width}" height="${gBox.height}" rx="14" fill="rgba(15,23,42,0.45)" stroke="${stroke}" stroke-width="2" stroke-dasharray="8 5"/>`
    );
    lines.push(
      `<text x="${gBox.x + 14}" y="${gBox.y + 22}" fill="${stroke}" font-size="12" font-weight="bold" font-family="monospace">${esc(
        gBox.group.label
      )} [${esc(gBox.group.provider.toUpperCase())} • ${esc(gBox.group.resource_type)}]</text>`
    );
  }

  // Edges
  for (const rfe of rfEdges) {
    const s = nodeBoxes.get(rfe.source);
    const t = nodeBoxes.get(rfe.target);
    if (!s || !t) continue;
    const sx = s.x + s.width;
    const sy = s.y + s.height / 2;
    const tx = t.x;
    const ty = t.y + t.height / 2;
    const mx = (sx + tx) / 2;
    const edgeData = (rfe.data as { edge?: PipelineEdge } | undefined)?.edge;
    const relType: RelationType = edgeData?.relation_type ?? 'triggers';
    const col = RELATION_COLORS[relType] ?? '#38bdf8';
    const lbl = String(rfe.label ?? edgeData?.label ?? relType);

    lines.push(
      `<path d="M ${sx} ${sy} C ${mx} ${sy}, ${mx} ${ty}, ${tx} ${ty}" fill="none" stroke="${col}" stroke-width="2"/>`
    );
    lines.push(
      `<text x="${mx}" y="${(sy + ty) / 2 - 6}" fill="#e2e8f0" font-size="10" font-family="monospace" text-anchor="middle">${esc(
        lbl
      )}</text>`
    );
  }

  // Nodes
  for (const rfn of rfNodes) {
    if (rfn.type !== 'devopsNode') continue;
    const box = nodeBoxes.get(rfn.id);
    if (!box) continue;
    const node = (rfn.data as unknown as DevOpsNodeData).node;
    const theme = TOOL_ACCENT_HEX[node.tool] ?? TOOL_ACCENT_HEX.github;
    const subtitle = extractNodeSubtitle(node);
    const tags = extractNodeDisplayTags(node);

    lines.push(
      `<g transform="translate(${box.x},${box.y})">`,
      `<rect width="${box.width}" height="${box.height}" rx="10" fill="#0f172a" stroke="${theme.main}" stroke-width="1.8"/>`,
      `<text x="14" y="22" fill="${theme.main}" font-size="10" font-weight="bold" font-family="monospace">${esc(
        theme.label
      )} • ${esc(node.stage.toUpperCase())} [${esc(node.status.toUpperCase())}]</text>`,
      `<text x="14" y="44" fill="#f8fafc" font-size="13" font-weight="bold">${esc(
        node.label
      )}</text>`,
      `<text x="14" y="62" fill="#94a3b8" font-size="10" font-family="monospace">${esc(
        subtitle.slice(0, 40)
      )}</text>`,
      `<text x="14" y="86" fill="#67e8f9" font-size="10" font-family="monospace">${esc(
        tags.slice(0, 3).join('  |  ')
      )}</text>`,
      `</g>`
    );
  }

  lines.push(`</svg>`);
  const blob = new Blob([lines.join('\n')], { type: 'image/svg+xml;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${graph.id || 'pipescope-pipeline'}-architecture.svg`;
  a.click();
  URL.revokeObjectURL(url);
}

// ============================================================================
// 3. Zero-Dependency GIF89a Encoder & Animated Pipeline Flow Exporter
// ============================================================================

function buildGifPalette(): { paletteBytes: Uint8Array; rgbList: [number, number, number][] } {
  const rgbList: [number, number, number][] = [];

  const exactHexColors = [
    '#020617', '#0f172a', '#1e293b', '#334155', '#475569', '#64748b', '#94a3b8', '#cbd5e1',
    '#e2e8f0', '#f8fafc', '#22d3ee', '#67e8f9', '#083344', '#164e63', '#38bdf8', '#082f49',
    '#60a5fa', '#172554', '#818cf8', '#1e1b4b', '#c084fc', '#3b0764', '#f472b6', '#fb7185',
    '#4c0519', '#f43f5e', '#fb923c', '#431407', '#fbbf24', '#f59e0b', '#451a03', '#fde68a',
    '#10b981', '#34d399', '#6ee7b7', '#064e3b', '#2dd4bf', '#c7d2fe', '#fda4af', '#fcd34d',
  ];

  for (const hex of exactHexColors) {
    const r = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    rgbList.push([r, g, b]);
  }

  const levels = [0, 51, 102, 153, 204, 255];
  for (const r of levels) {
    for (const g of levels) {
      for (const b of levels) {
        if (rgbList.length < 256) {
          rgbList.push([r, g, b]);
        }
      }
    }
  }

  while (rgbList.length < 256) {
    rgbList.push([0, 0, 0]);
  }

  const paletteBytes = new Uint8Array(256 * 3);
  for (let i = 0; i < 256; i++) {
    paletteBytes[i * 3] = rgbList[i][0];
    paletteBytes[i * 3 + 1] = rgbList[i][1];
    paletteBytes[i * 3 + 2] = rgbList[i][2];
  }

  return { paletteBytes, rgbList };
}

function quantizeFrameToIndices(
  rgba: Uint8ClampedArray,
  width: number,
  height: number,
  rgbList: [number, number, number][]
): Uint8Array {
  const total = width * height;
  const indexed = new Uint8Array(total);
  const cache = new Int16Array(32768).fill(-1);

  for (let i = 0; i < total; i++) {
    const off = i * 4;
    const r = rgba[off];
    const g = rgba[off + 1];
    const b = rgba[off + 2];
    const key = ((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3);

    let bestIdx = cache[key];
    if (bestIdx === -1) {
      let minDist = Infinity;
      bestIdx = 0;
      for (let p = 0; p < 256; p++) {
        const pr = rgbList[p][0] - r;
        const pg = rgbList[p][1] - g;
        const pb = rgbList[p][2] - b;
        const dist = pr * pr + pg * pg + pb * pb;
        if (dist < minDist) {
          minDist = dist;
          bestIdx = p;
          if (dist === 0) break;
        }
      }
      cache[key] = bestIdx;
    }
    indexed[i] = bestIdx;
  }

  return indexed;
}

function lzwEncodeGifFrame(indices: Uint8Array): Uint8Array {
  const minCodeSize = 8;
  const clearCode = 1 << minCodeSize; // 256
  const eoiCode = clearCode + 1;      // 257

  const outBytes: number[] = [minCodeSize];
  const subBlock: number[] = [];

  let bitBuffer = 0;
  let bitCount = 0;

  const flushSubBlock = () => {
    if (subBlock.length > 0) {
      outBytes.push(subBlock.length);
      for (let i = 0; i < subBlock.length; i++) {
        outBytes.push(subBlock[i]);
      }
      subBlock.length = 0;
    }
  };

  const writeCode = (code: number, codeSize: number) => {
    bitBuffer |= code << bitCount;
    bitCount += codeSize;
    while (bitCount >= 8) {
      subBlock.push(bitBuffer & 0xff);
      if (subBlock.length === 255) {
        flushSubBlock();
      }
      bitBuffer >>= 8;
      bitCount -= 8;
    }
  };

  let codeSize = minCodeSize + 1; // 9
  let nextCode = eoiCode + 1;     // 258
  const dict = new Int32Array(4096 * 256).fill(-1);

  writeCode(clearCode, codeSize);

  if (indices.length > 0) {
    let prefix = indices[0];
    for (let i = 1; i < indices.length; i++) {
      const k = indices[i];
      const dictKey = (prefix << 8) | k;
      const existing = dict[dictKey];
      if (existing !== -1) {
        prefix = existing;
      } else {
        writeCode(prefix, codeSize);
        if (nextCode < 4096) {
          dict[dictKey] = nextCode;
          nextCode++;
          if (nextCode > 1 << codeSize && codeSize < 12) {
            codeSize++;
          }
        } else {
          writeCode(clearCode, codeSize);
          dict.fill(-1);
          codeSize = minCodeSize + 1;
          nextCode = eoiCode + 1;
        }
        prefix = k;
      }
    }
    writeCode(prefix, codeSize);
  }

  writeCode(eoiCode, codeSize);
  if (bitCount > 0) {
    subBlock.push(bitBuffer & 0xff);
  }
  flushSubBlock();
  outBytes.push(0);

  return new Uint8Array(outBytes);
}

export async function exportPipelineGIF(
  graph: PipelineGraph,
  rfNodes: RFNode[],
  rfEdges: RFEdge[]
): Promise<void> {
  const width = 1280;
  const height = 740;
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  if (!ctx) return;

  const { paletteBytes, rgbList } = buildGifPalette();
  const chunks: Uint8Array[] = [];

  const pushBytes = (...bytes: number[]) => {
    chunks.push(new Uint8Array(bytes));
  };
  const pushU16LE = (val: number) => {
    pushBytes(val & 0xff, (val >> 8) & 0xff);
  };

  // 1. GIF89a Header & Logical Screen Descriptor
  pushBytes(0x47, 0x49, 0x46, 0x38, 0x39, 0x61); // "GIF89a"
  pushU16LE(width);
  pushU16LE(height);
  pushBytes(0xf7, 0x00, 0x00);
  chunks.push(paletteBytes);

  // 2. Netscape 2.0 Loop Extension
  pushBytes(
    0x21, 0xff, 0x0b,
    0x4e, 0x45, 0x54, 0x53, 0x43, 0x41, 0x50, 0x45, 0x32, 0x2e, 0x30,
    0x03, 0x01, 0x00, 0x00, 0x00
  );

  const phases: {
    name: string;
    banner: string;
    tools: Set<Tool>;
  }[] = [
    {
      name: 'source',
      banner: 'STAGE 1/6: SOURCE WEBHOOK & GIT TRIGGER',
      tools: new Set(['github']),
    },
    {
      name: 'ci',
      banner: 'STAGE 2/6: GITHUB ACTIONS CI BUILD & TEST',
      tools: new Set(['github_actions']),
    },
    {
      name: 'artifact',
      banner: 'STAGE 3/6: DOCKER IMAGE BUILD & REGISTRY PUSH',
      tools: new Set(['docker']),
    },
    {
      name: 'iac_and_config_mgmt',
      banner: 'STAGE 4/6: TERRAFORM IAC & ANSIBLE PROVISIONING',
      tools: new Set(['terraform', 'ansible']),
    },
    {
      name: 'gitops',
      banner: 'STAGE 5/6: ARGO CD GITOPS MANIFEST SYNC',
      tools: new Set(['argocd']),
    },
    {
      name: 'runtime_and_cloud',
      banner: 'STAGE 6/6: KUBERNETES RUNTIME & CLOUD WORKLOADS',
      tools: new Set(['kubernetes', 'aws', 'azure']),
    },
  ];

  const completedTools = new Set<Tool>();

  const appendEncodedFrame = (delayCentiseconds: number) => {
    const imgData = ctx.getImageData(0, 0, width, height);
    const indices = quantizeFrameToIndices(imgData.data, width, height, rgbList);
    const lzwData = lzwEncodeGifFrame(indices);

    pushBytes(0x21, 0xf9, 0x04, 0x00);
    pushU16LE(delayCentiseconds);
    pushBytes(0x00, 0x00);

    pushBytes(0x2c);
    pushU16LE(0);
    pushU16LE(0);
    pushU16LE(width);
    pushU16LE(height);
    pushBytes(0x00);

    chunks.push(lzwData);
  };

  for (let pIdx = 0; pIdx < phases.length; pIdx++) {
    const phase = phases[pIdx];
    const statusMap: Record<string, NodeStatus> = {};
    const runningNodeIds = new Set<string>();

    for (const n of graph.nodes) {
      if (phase.tools.has(n.tool)) {
        statusMap[n.id] = 'running';
        runningNodeIds.add(n.id);
      } else if (completedTools.has(n.tool)) {
        statusMap[n.id] = 'success';
      } else {
        statusMap[n.id] = 'idle';
      }
    }

    for (const rfn of rfNodes) {
      if (rfn.id.startsWith('stage-summary-')) {
        const nd = (rfn.data as unknown as DevOpsNodeData).node;
        if (phase.tools.has(nd.tool)) {
          statusMap[rfn.id] = 'running';
          runningNodeIds.add(rfn.id);
        } else if (completedTools.has(nd.tool)) {
          statusMap[rfn.id] = 'success';
        } else {
          statusMap[rfn.id] = 'idle';
        }
      }
    }

    const activeEdges = new Set<string>();
    for (const rfe of rfEdges) {
      if (runningNodeIds.has(rfe.source) || runningNodeIds.has(rfe.target)) {
        activeEdges.add(rfe.id);
      }
    }

    for (const progress of [0.22, 0.72]) {
      renderPipelineFrameToCanvas(ctx, {
        width,
        height,
        graph,
        rfNodes,
        rfEdges,
        statusOverrides: statusMap,
        activeEdgeIds: activeEdges,
        particleProgress: progress,
        phaseBanner: phase.banner,
        phaseIndex: pIdx + 1,
      });
      appendEncodedFrame(38);
      await new Promise((r) => setTimeout(r, 0));
    }

    for (const t of phase.tools) {
      completedTools.add(t);
    }
  }

  const finalStatusMap: Record<string, NodeStatus> = {};
  for (const n of graph.nodes) {
    finalStatusMap[n.id] = 'success';
  }
  for (const rfn of rfNodes) {
    if (rfn.id.startsWith('stage-summary-')) {
      finalStatusMap[rfn.id] = 'success';
    }
  }

  renderPipelineFrameToCanvas(ctx, {
    width,
    height,
    graph,
    rfNodes,
    rfEdges,
    statusOverrides: finalStatusMap,
    activeEdgeIds: new Set(),
    particleProgress: 1,
    phaseBanner: 'PIPELINE EXECUTION COMPLETED SUCCESSFULLY ✓',
    phaseIndex: 6,
  });
  appendEncodedFrame(120);

  pushBytes(0x3b);

  const blob = new Blob(chunks, { type: 'image/gif' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${graph.id || 'pipescope-pipeline'}-live-flow.gif`;
  a.click();
  URL.revokeObjectURL(url);
}
