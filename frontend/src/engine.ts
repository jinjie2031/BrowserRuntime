export interface EngineStatus { enabled: boolean; ready: boolean; phase: string; received: number; total: number }
export interface EngineAPI { getStatus(): Promise<EngineStatus>; download(): Promise<void>; cancel(): Promise<void> }

function binding() {
  return (window as unknown as { go?: { main?: { EngineBootstrap?: { GetStatus(): Promise<EngineStatus>; Download(): Promise<void>; Cancel(): Promise<void> } } } }).go?.main?.EngineBootstrap;
}
export const engineBackend: EngineAPI = {
  async getStatus() {
    const host = binding();
    if (!host) return { enabled: true, ready: false, phase: 'FAILED', received: 0, total: 0 };
    return host.GetStatus();
  },
  async download() { const host = binding(); if (!host) throw new Error('Engine unavailable'); await host.Download(); },
  async cancel() { await binding()?.Cancel(); },
};
