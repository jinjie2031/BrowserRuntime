import { useEffect, useState } from 'react';
import type { NetworkView, ProfileAPI, SetProxyInput } from '../types';
import { diagnosticError } from '../diagnostics';
import { useLanguage } from '../i18n';

interface Props {
  profileID: string;
  api: ProfileAPI;
  running: boolean;
  blockedReason?: string;
  onChanged(): unknown;
}

export function NetworkPanel({ profileID, api, running, blockedReason, onChanged }: Props) {
  const { t, label } = useLanguage();
  const [view, setView] = useState<NetworkView | null>(null);
  const [mode, setMode] = useState<'direct' | 'proxy'>('direct');
  const [scheme, setScheme] = useState<SetProxyInput['scheme']>('http');
  const [host, setHost] = useState('');
  const [port, setPort] = useState('');
  const [username, setUsername] = useState('');
  const [replacement, setReplacement] = useState('');
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  function apply(next: NetworkView) {
    setView(next);
    setMode(next.mode);
    setScheme(next.scheme || 'http');
    setHost(next.host || '');
    setPort(next.port ? String(next.port) : '');
    setUsername(next.username || '');
    setReplacement('');
    setDirty(false);
  }

  useEffect(() => {
    let active = true;
    void api.getNetwork(profileID).then((next) => { if (active) apply(next); }).catch((reason) => { if (active) setError(diagnosticError(reason)); });
    return () => { active = false; };
  }, [api, profileID]);

  function edit(change: () => void) { change(); setDirty(true); setError(''); }

  async function save() {
    setBusy(true); setError('');
    try {
      if (mode === 'proxy' && (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535)) throw new Error('Port must be between 1 and 65535.');
      const next = mode === 'direct'
        ? await api.setDirect(profileID)
        : await api.setProxy(profileID, { scheme, host: host.trim(), port: Number(port), username, password: replacement });
      apply(next);
      await onChanged();
    } catch (reason) { setError(diagnosticError(reason)); }
    finally { setReplacement(''); setBusy(false); }
  }

  async function test() {
    setBusy(true); setError('');
    try { apply(await api.testNetwork(profileID)); await onChanged(); }
    catch (reason) {
      setView((current) => current ? { ...current, status: 'Connection Failed', locality: undefined } : current);
      setError(diagnosticError(reason));
      await onChanged();
    }
    finally { setBusy(false); }
  }

  if (!view) return error ? <div role="alert" className="error-banner">{t(error)}</div> : <p role="status">{t('Loading network…')}</p>;
  const status = dirty ? (mode === 'proxy' ? 'Needs Test' : 'Direct') : view.status;
  const locality = !dirty ? view.locality : undefined;

  return (
    <div className="network-panel">
      <fieldset className="network-fields" disabled={busy || running}>
        <legend>{t('Mode')}</legend>
        <div className="mode-options">
          <label><input type="radio" name="network-mode" value="direct" checked={mode === 'direct'} onChange={() => edit(() => setMode('direct'))} /> {t('Direct')}</label>
          <label><input type="radio" name="network-mode" value="proxy" checked={mode === 'proxy'} onChange={() => edit(() => setMode('proxy'))} /> {t('Proxy')}</label>
        </div>
        {mode === 'proxy' && <div className="proxy-fields">
          <label>{t('Protocol')}<select value={scheme} onChange={(event) => edit(() => setScheme(event.target.value as SetProxyInput['scheme']))}><option value="http">HTTP</option><option value="https">HTTPS</option><option value="socks5">SOCKS5</option></select></label>
          <label>{t('Host')}<input value={host} onChange={(event) => edit(() => setHost(event.target.value))} autoComplete="off" spellCheck={false} /></label>
          <label>{t('Port')}<input type="number" min="1" max="65535" value={port} onChange={(event) => edit(() => setPort(event.target.value))} /></label>
          <label>{t('Username')}<input value={username} onChange={(event) => edit(() => setUsername(event.target.value))} autoComplete="off" spellCheck={false} /></label>
          <label>{t('Password')}<input type="password" value={replacement} onChange={(event) => edit(() => setReplacement(event.target.value))} placeholder={t(view.has_password ? 'Saved password — leave blank to keep' : 'Enter proxy password')} autoComplete="new-password" /></label>
        </div>}
      </fieldset>
      {running && <p className="field-hint">{t(blockedReason || 'Stop the browser to change or test its network.')}</p>}
      {error && <div role="alert" className="error-banner">{t(error)}</div>}
      <div className="network-actions">
        <button className="button primary" disabled={busy || running} onClick={() => void save()}>{t('Save')}</button>
        <button className="button secondary" disabled={busy || running || dirty || mode === 'direct'} onClick={() => void test()}>{t('Test Connection')}</button>
      </div>
      {dirty && <p className="field-hint">{t('Save changes before testing.')}</p>}
      {busy && <p role="status" className="field-hint">{t('Working…')}</p>}
      <section className="connection-section" aria-labelledby="connection-title">
        <h3 id="connection-title">{t('Connection')}</h3>
        <p className={`connection-status ${status === 'Ready' ? 'ready' : status === 'Connection Failed' ? 'failed' : ''}`} aria-live="polite">{label(status)}</p>
        {locality && <dl className="detail-list">
          <div><dt>{t('Exit IP')}</dt><dd>{locality.exit_ip}</dd></div>
          <div><dt>{t('Country')}</dt><dd>{locality.country_code}</dd></div>
          <div><dt>{t('City')}</dt><dd>{locality.city || t('Not provided')}</dd></div>
          <div><dt>{t('Timezone')}</dt><dd>{locality.timezone}</dd></div>
          <div><dt>{t('Locale')}</dt><dd>{locality.locale}</dd></div>
        </dl>}
      </section>
    </div>
  );
}
