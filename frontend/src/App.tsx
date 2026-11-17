import React, { useEffect } from 'react';
import { HeaderBar } from './components/HeaderBar';
import { ToolboxSidebar } from './components/ToolboxSidebar';
import { PipelineCanvas } from './components/PipelineCanvas';
import { NodeInspector } from './components/NodeInspector';
import { usePipelineStore } from './store/usePipelineStore';

export const App: React.FC = () => {
  const { loadSamplePipeline, connectWebSocket } = usePipelineStore();

  useEffect(() => {
    void loadSamplePipeline();
    const disconnect = connectWebSocket();
    return disconnect;
  }, [loadSamplePipeline, connectWebSocket]);

  return (
    <div className="flex flex-col w-screen h-screen bg-slate-950 text-slate-100 overflow-hidden">
      {/* Top Action Header */}
      <HeaderBar />

      {/* Main 3-Column Interactive Workbench: Left Toolbox + Center DAG Canvas + Right Node Inspector */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <ToolboxSidebar />
        <main className="flex-1 relative min-w-0 min-h-0">
          <PipelineCanvas />
        </main>
        <NodeInspector />
      </div>
    </div>
  );
};

export default App;
