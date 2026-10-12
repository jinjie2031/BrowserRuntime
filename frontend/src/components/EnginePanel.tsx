import { useEffect, useState } from 'react';
import type { EngineAPI, EngineStatus } from '../engine';
import { useLanguage } from '../i18n';

const messages: Record<string, string> = {
  CHECKING: 'Checking CloakBrowser engine…',
  MISSING: 'CloakBrowser engine is not installed',
  DOWNLOADING: 'Downloading official engine…',
  EXTRACTING: 'Verifying and installing engine…',
  CANCELLED: 'Engine download cancelled. Profiles cannot launch yet.',
  FAILED: 'Official engine download failed. Please retry.',
};

export function EnginePanel({ api, onReady }: { api: EngineAPI; onReady(ready: boolean): void }) {
  const { t } = useLanguage();
  const [status, setStatus] = useState<EngineStatus | null>(null);
  const [dismissed, setDismissed] = useState(false);
  useEffect(() => {
    let active = true;
    async function refresh() {
      try {
        const value = await api.getStatus();
        if (active) { setStatus(value); onReady(value.ready); }
      } catch {
        if (active) { setStatus({ enabled: true, ready: false, phase: 'FAILED', received: 0, total: 0 }); onReady(false); }
      }
    }
    void refresh();
    const timer = window.setInterval(() => void refresh(), 1000);
    return () => { active = false; window.clearInterval(timer); };
  }, [api, onReady]);

  if (!status || !status.enabled || status.ready) return null;
  const busy = status.phase === 'DOWNLOADING' || status.phase === 'EXTRACTING';
  async function download() {
    setDismissed(false);
    try { await api.download(); }
    catch { setStatus({ enabled: true, ready: false, phase: 'FAILED', received: 0, total: 0 }); }
  }
  async function cancel() {
    try { await api.cancel(); } catch { /* No raw diagnostic data. */ }
  }
  return <section className="engine-panel" aria-label={t('Browser engine')}>
    <div role="status">
      <strong>{t(messages[status.phase] ?? 'CloakBrowser engine is not ready')}</strong>
      {!dismissed && <p>{t('Download the official browser engine to launch profiles.')}</p>}
      {status.phase === 'DOWNLOADING' && <>
        <progress aria-label={t('Engine download progress')} value={status.received} max={status.total > 0 ? status.total : 734003200} />
        <span className="engine-progress">{Math.floor(status.received / 1048576)} / {status.total > 0 ? Math.ceil(status.total / 1048576) : '—'} MB</span>
      </>}
    </div>
    <div className="toolbar-actions">
      {busy ? <button className="button secondary" onClick={() => void cancel()}>{t('Cancel download')}</button> : status.phase !== 'CHECKING' && <>
        <button className="button primary" onClick={() => void download()}>{t(status.phase === 'FAILED' ? 'Retry download' : 'Download and install')}</button>
        {!dismissed && <button className="button secondary" onClick={() => setDismissed(true)}>{t('Cancel')}</button>}
      </>}
    </div>
  </section>;
}
