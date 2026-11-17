import React, { useState } from 'react';
import {
  Download,
  FileUp,
  FolderGit2,
  Image as ImageIcon,
  LayoutTemplate,
  Play,
  RefreshCw,
  Upload,
  Workflow,
  X,
} from 'lucide-react';
import {
  usePipelineStore,
  type UploadedFilePayload,
} from '../store/usePipelineStore';

export const HeaderBar: React.FC = () => {
  const {
    graph,
    isLoading,
    isSimulating,
    loadSamplePipeline,
    triggerSimulation,
    autoLayout,
    scanGitHubRepo,
    parseUploadedFiles,
    error,
  } = usePipelineStore();

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<'github' | 'upload'>('github');

  // GitHub Repo Scan State
  const [repoUrl, setRepoUrl] = useState('https://github.com/pipescope/sample-pipeline');
  const [branch, setBranch] = useState('main');
  const [githubToken, setGithubToken] = useState('');

  // Custom Upload / Snippet State
  const [uploadedFiles, setUploadedFiles] = useState<UploadedFilePayload[]>([
    {
      path: '.github/workflows/deploy.yml',
      content: `name: Custom CI/CD\non:\n  push:\n    branches: [main]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/build-push-action@v5\n        with:\n          tags: ghcr.io/org/custom-api:latest`,
    },
    {
      path: 'k8s/deployment.yaml',
      content: `apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: custom-api\nspec:\n  template:\n    spec:\n      containers:\n        - name: api\n          image: ghcr.io/org/custom-api:latest`,
    },
  ]);

  const handleExportJSON = () => {
    if (!graph) return;
    const blob = new Blob([JSON.stringify(graph, null, 2)], {
      type: 'application/json',
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${graph.id || 'pipescope-pipeline'}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleExportPNG = () => {
    const svgEl = document.querySelector('.react-flow__edges') as SVGSVGElement | null;
    if (!svgEl && !graph) return;

    // Generate a clean high-resolution summary card + DAG snapshot PNG using HTML5 Canvas
    const canvas = document.createElement('canvas');
    canvas.width = 1600;
    canvas.height = 900;
    const ctx = canvas.getContext('2d');
    if (!ctx || !graph) return;

    ctx.fillStyle = '#020617';
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    ctx.fillStyle = '#22d3ee';
    ctx.font = 'bold 24px Inter, sans-serif';
    ctx.fillText(`PipeScope — ${graph.name}`, 40, 48);

    ctx.fillStyle = '#94a3b8';
    ctx.font = '14px monospace';
    ctx.fillText(
      `Nodes: ${graph.nodes.length} | Edges: ${graph.edges.length} | Groups: ${graph.groups.length} | Updated: ${graph.updated_at}`,
      40,
      76
    );

    graph.nodes.forEach((n, idx) => {
      const x = n.position.x > 0 ? n.position.x * 0.75 + 60 : 60 + (idx % 5) * 290;
      const y = n.position.y > 0 ? n.position.y * 0.75 + 120 : 130 + Math.floor(idx / 5) * 140;

      ctx.fillStyle = '#0f172a';
      ctx.strokeStyle = '#38bdf8';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.roundRect(x, y, 240, 82, 12);
      ctx.fill();
      ctx.stroke();

      ctx.fillStyle = '#38bdf8';
      ctx.font = 'bold 11px monospace';
      ctx.fillText(`${n.tool.toUpperCase()} • ${n.stage.toUpperCase()}`, x + 14, y + 26);

      ctx.fillStyle = '#f8fafc';
      ctx.font = 'bold 13px Inter, sans-serif';
      ctx.fillText(n.label.slice(0, 28), x + 14, y + 50);

      ctx.fillStyle = '#64748b';
      ctx.font = '11px monospace';
      ctx.fillText(n.id.slice(0, 30), x + 14, y + 68);
    });

    const dataUrl = canvas.toDataURL('image/png');
    const a = document.createElement('a');
    a.href = dataUrl;
    a.download = `${graph.id || 'pipescope-architecture'}.png`;
    a.click();
  };

  const handleFileInputChange = async (
    e: React.ChangeEvent<HTMLInputElement>
  ) => {
    const fileList = e.target.files;
    if (!fileList || fileList.length === 0) return;

    const loaded: UploadedFilePayload[] = [];
    for (const file of Array.from(fileList)) {
      const text = await file.text();
      const relPath =
        (file as File & { webkitRelativePath?: string }).webkitRelativePath ||
        file.name;
      loaded.push({ path: relPath, content: text });
    }
    setUploadedFiles((prev) => [...prev, ...loaded]);
  };

  const handleRunGitHubScan = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await scanGitHubRepo(repoUrl, branch, githubToken);
      setIsModalOpen(false);
    } catch {
      // Error displayed in modal
    }
  };

  const handleRunUploadParse = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await parseUploadedFiles(uploadedFiles);
      setIsModalOpen(false);
    } catch {
      // Error displayed in modal
    }
  };

  return (
    <>
      <header className="h-14 shrink-0 px-5 border-b border-slate-800/90 bg-slate-900/85 backdrop-blur-md flex items-center justify-between z-30">
        {/* Brand Title */}
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-xl bg-cyan-500/15 border border-cyan-400/40 text-cyan-300 shadow-glow-cyan">
            <Workflow className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-sm font-bold tracking-tight text-white">
                PipeScope
              </h1>
              <span className="text-[10px] font-mono uppercase px-2 py-0.5 rounded-full bg-slate-800 text-cyan-300 border border-slate-700">
                CI/CD & Cloud DAG Visualizer
              </span>
            </div>
            <p className="text-[11px] text-slate-400">
              {graph
                ? `${graph.name} • ${graph.nodes.length} nodes • ${graph.edges.length} edges`
                : 'Loading pipeline topology...'}
            </p>
          </div>
        </div>

        {/* Action Buttons */}
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setIsModalOpen(true)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium bg-slate-800/90 hover:bg-slate-700 text-cyan-300 border border-cyan-500/30 transition-colors"
          >
            <FolderGit2 className="w-3.5 h-3.5" />
            <span>Scan GitHub Repo</span>
          </button>

          <button
            type="button"
            onClick={() => void loadSamplePipeline()}
            disabled={isLoading}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium bg-slate-800/90 hover:bg-slate-700 text-slate-200 border border-slate-700 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin' : ''}`} />
            <span>Sample Pipeline</span>
          </button>

          <button
            type="button"
            onClick={() => void autoLayout()}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium bg-slate-800/90 hover:bg-slate-700 text-slate-200 border border-slate-700 transition-colors"
            title="Recompute hierarchical ELK layout"
          >
            <LayoutTemplate className="w-3.5 h-3.5" />
            <span>Auto-Layout</span>
          </button>

          <button
            type="button"
            onClick={handleExportJSON}
            className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-xl text-xs font-medium bg-slate-800/90 hover:bg-slate-700 text-slate-300 border border-slate-700 transition-colors"
            title="Export PipelineGraph JSON"
          >
            <Download className="w-3.5 h-3.5" />
            <span>JSON</span>
          </button>

          <button
            type="button"
            onClick={handleExportPNG}
            className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-xl text-xs font-medium bg-slate-800/90 hover:bg-slate-700 text-slate-300 border border-slate-700 transition-colors"
            title="Export Architecture PNG"
          >
            <ImageIcon className="w-3.5 h-3.5" />
            <span>PNG</span>
          </button>

          <button
            type="button"
            onClick={() => void triggerSimulation()}
            disabled={isSimulating}
            className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl text-xs font-semibold bg-cyan-500 hover:bg-cyan-400 disabled:bg-cyan-500/40 text-slate-950 shadow-glow-cyan transition-all"
          >
            <Play className="w-3.5 h-3.5 fill-current" />
            <span>
              {isSimulating ? 'Simulating Live Run...' : 'Simulate Live CI/CD Run'}
            </span>
          </button>
        </div>
      </header>

      {/* Scan GitHub Repo / Upload Configs Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 bg-slate-950/80 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-2xl rounded-2xl bg-slate-900 border border-slate-800 shadow-2xl overflow-hidden">
            <div className="px-5 py-4 border-b border-slate-800 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <FolderGit2 className="w-5 h-5 text-cyan-400" />
                <h2 className="text-sm font-bold text-white">
                  Import & Correlate DevOps Pipeline
                </h2>
              </div>
              <button
                type="button"
                onClick={() => setIsModalOpen(false)}
                className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Modal Tabs */}
            <div className="flex border-b border-slate-800 bg-slate-950/50 px-5 pt-2 gap-2">
              <button
                type="button"
                onClick={() => setActiveTab('github')}
                className={`px-4 py-2 text-xs font-semibold rounded-t-xl border-b-2 transition-colors ${
                  activeTab === 'github'
                    ? 'border-cyan-400 text-cyan-300 bg-slate-900'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                Scan GitHub Repository
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('upload')}
                className={`px-4 py-2 text-xs font-semibold rounded-t-xl border-b-2 transition-colors ${
                  activeTab === 'upload'
                    ? 'border-cyan-400 text-cyan-300 bg-slate-900'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                Upload / Edit Config Files (.yml, .tf, Dockerfile)
              </button>
            </div>

            {error && (
              <div className="mx-5 mt-4 p-3 rounded-xl bg-rose-950/60 border border-rose-500/40 text-xs text-rose-200">
                {error}
              </div>
            )}

            {activeTab === 'github' ? (
              <form onSubmit={(e) => void handleRunGitHubScan(e)} className="p-5 space-y-4">
                <div>
                  <label className="block text-xs font-medium text-slate-300 mb-1">
                    GitHub Repository URL
                  </label>
                  <input
                    type="text"
                    required
                    value={repoUrl}
                    onChange={(e) => setRepoUrl(e.target.value)}
                    placeholder="https://github.com/owner/repo"
                    className="w-full px-3 py-2 rounded-xl bg-slate-950 border border-slate-800 text-xs text-slate-100 focus:outline-none focus:border-cyan-400"
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">
                      Branch
                    </label>
                    <input
                      type="text"
                      value={branch}
                      onChange={(e) => setBranch(e.target.value)}
                      placeholder="main"
                      className="w-full px-3 py-2 rounded-xl bg-slate-950 border border-slate-800 text-xs text-slate-100 focus:outline-none focus:border-cyan-400"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">
                      GitHub Token (Optional, for private repos)
                    </label>
                    <input
                      type="password"
                      value={githubToken}
                      onChange={(e) => setGithubToken(e.target.value)}
                      placeholder="ghp_..."
                      className="w-full px-3 py-2 rounded-xl bg-slate-950 border border-slate-800 text-xs text-slate-100 focus:outline-none focus:border-cyan-400"
                    />
                  </div>
                </div>

                <div className="flex justify-end gap-2 pt-2">
                  <button
                    type="button"
                    onClick={() => setIsModalOpen(false)}
                    className="px-4 py-2 rounded-xl text-xs font-medium bg-slate-800 text-slate-300 hover:bg-slate-700"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={isLoading}
                    className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold bg-cyan-500 hover:bg-cyan-400 text-slate-950"
                  >
                    <FolderGit2 className="w-3.5 h-3.5" />
                    <span>{isLoading ? 'Scanning Repo...' : 'Scan & Visualize'}</span>
                  </button>
                </div>
              </form>
            ) : (
              <form onSubmit={(e) => void handleRunUploadParse(e)} className="p-5 space-y-4">
                <div className="flex items-center justify-between">
                  <label className="inline-flex items-center gap-2 px-3 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-xs font-medium text-cyan-300 cursor-pointer border border-slate-700">
                    <FileUp className="w-4 h-4" />
                    <span>Select Local Config Files</span>
                    <input
                      type="file"
                      multiple
                      onChange={(e) => void handleFileInputChange(e)}
                      className="hidden"
                    />
                  </label>
                  <button
                    type="button"
                    onClick={() =>
                      setUploadedFiles((prev) => [
                        ...prev,
                        { path: 'terraform/main.tf', content: 'resource "aws_vpc" "main" {\n  cidr_block = "10.0.0.0/16"\n}' },
                      ])
                    }
                    className="text-xs text-cyan-400 hover:underline"
                  >
                    + Add Empty File Entry
                  </button>
                </div>

                <div className="max-h-64 overflow-y-auto space-y-3 pr-1">
                  {uploadedFiles.map((file, idx) => (
                    <div
                      key={idx}
                      className="p-3 rounded-xl bg-slate-950 border border-slate-800 space-y-2"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <input
                          type="text"
                          value={file.path}
                          onChange={(e) => {
                            const val = e.target.value;
                            setUploadedFiles((prev) =>
                              prev.map((item, i) =>
                                i === idx ? { ...item, path: val } : item
                              )
                            );
                          }}
                          placeholder="e.g. .github/workflows/ci.yml or Dockerfile"
                          className="flex-1 px-2.5 py-1 rounded-lg bg-slate-900 border border-slate-800 text-xs font-mono text-cyan-300"
                        />
                        <button
                          type="button"
                          onClick={() =>
                            setUploadedFiles((prev) =>
                              prev.filter((_, i) => i !== idx)
                            )
                          }
                          className="text-xs text-rose-400 hover:underline"
                        >
                          Remove
                        </button>
                      </div>
                      <textarea
                        rows={3}
                        value={file.content}
                        onChange={(e) => {
                          const val = e.target.value;
                          setUploadedFiles((prev) =>
                            prev.map((item, i) =>
                              i === idx ? { ...item, content: val } : item
                            )
                          );
                        }}
                        className="w-full p-2 rounded-lg bg-slate-900 border border-slate-800 text-[11px] font-mono text-slate-200 focus:outline-none focus:border-cyan-400"
                      />
                    </div>
                  ))}
                </div>

                <div className="flex justify-end gap-2 pt-2">
                  <button
                    type="button"
                    onClick={() => setIsModalOpen(false)}
                    className="px-4 py-2 rounded-xl text-xs font-medium bg-slate-800 text-slate-300 hover:bg-slate-700"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={isLoading || uploadedFiles.length === 0}
                    className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold bg-cyan-500 hover:bg-cyan-400 text-slate-950"
                  >
                    <Upload className="w-3.5 h-3.5" />
                    <span>{isLoading ? 'Parsing...' : 'Parse & Stitch Graph'}</span>
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}
    </>
  );
};
