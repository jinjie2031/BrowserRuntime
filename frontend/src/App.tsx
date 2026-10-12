import { useCallback, useEffect, useMemo, useState } from 'react';
import { backend } from './backend';
import { DeleteDialog } from './components/DeleteDialog';
import { ProfileDialog } from './components/ProfileDialog';
import { ProfileTable } from './components/ProfileTable';
import { ProfileDrawer } from './components/ProfileDrawer';
import type { ProfileAPI, ProfileView } from './types';
import { diagnosticError } from './diagnostics';
import { LanguageProvider, useLanguage } from './i18n';
import { engineBackend, type EngineAPI } from './engine';
import { EnginePanel } from './components/EnginePanel';

interface AppProps {
  api?: ProfileAPI;
  pollInterval?: number;
  engineAPI?: EngineAPI;
}

type Editor = { mode: 'create' } | { mode: 'rename'; profile: ProfileView } | null;

export function App({ api = backend, pollInterval = 1500, engineAPI = engineBackend }: AppProps) {
  return <LanguageProvider><ProfilesApp api={api} pollInterval={pollInterval} engineAPI={engineAPI} /></LanguageProvider>;
}

function ProfilesApp({ api = backend, pollInterval = 1500, engineAPI = engineBackend }: AppProps) {
  const { t, language, toggle } = useLanguage();
  const [profiles, setProfiles] = useState<ProfileView[]>([]);
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [editor, setEditor] = useState<Editor>(null);
  const [deleteTarget, setDeleteTarget] = useState<ProfileView | null>(null);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [engineReady, setEngineReady] = useState(false);
  const selectedProfile = profiles.find((profile) => profile.id === selectedID);

  const refresh = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true);
    try {
      setProfiles(await api.listProfiles());
      setError('');
    } catch (reason) {
      setError(diagnosticError(reason));
    } finally {
      if (showLoading) setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    void refresh(true);
    const timer = window.setInterval(() => void refresh(), pollInterval);
    return () => window.clearInterval(timer);
  }, [pollInterval, refresh]);

  const visibleProfiles = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    const filtered = needle ? profiles.filter((profile) => profile.name.toLocaleLowerCase().includes(needle)) : profiles;
    return engineReady ? filtered : filtered.map(profile => ({ ...profile, launch_ready: false, launch_blocked_reason: 'CloakBrowser engine is not ready' }));
  }, [profiles, query, engineReady]);

  async function action(operation: () => Promise<unknown>) {
    try {
      await operation();
      setError('');
      await refresh();
    } catch (reason) {
      setError(diagnosticError(reason));
    }
  }

  return (
    <main className="page-shell">
      <header className="page-header">
        <div>
          <p className="eyebrow">{t('Local browser environments')}</p>
          <h1>{t('Browser Runtime')}</h1>
        </div>
        <button className="button secondary language-button" onClick={toggle} lang={language === 'en-US' ? 'zh-CN' : 'en-US'}>{t('Switch language')}</button>
      </header>

      <EnginePanel api={engineAPI} onReady={setEngineReady} />
      <section className="profiles-section" aria-labelledby="profiles-title">
        <div className="toolbar">
          <div>
            <h2 id="profiles-title">{t('Profiles')}</h2>
            <p>{t('Persistent CloakBrowser profiles on this computer.')}</p>
          </div>
          <div className="toolbar-actions">
            <label className="search-field">
              <span className="sr-only">{t('Search Profiles')}</span>
              <input type="search" placeholder={t('Search Profiles')} value={query} onChange={(event) => setQuery(event.target.value)} />
            </label>
            <button className="button primary" onClick={() => setEditor({ mode: 'create' })}>{t('+ New Profile')}</button>
          </div>
        </div>

        {error && <div className="error-banner" role="alert">{t(error)}</div>}
        {loading ? <div className="loading-state" role="status">{t('Loading profiles…')}</div> : (
          <ProfileTable
            profiles={visibleProfiles}
            onLaunch={(profile) => void action(() => api.launchProfile(profile.id))}
            onOpen={(profile) => void action(() => api.openProfile(profile.id))}
            onStop={(profile) => void action(() => api.stopProfile(profile.id))}
            onRename={(profile) => setEditor({ mode: 'rename', profile })}
            onDelete={setDeleteTarget}
            onSelect={(profile) => setSelectedID(profile.id)}
          />
        )}
      </section>

      {selectedProfile && <ProfileDrawer profile={selectedProfile} api={api} onClose={() => setSelectedID(null)} onChanged={refresh} />}

      {editor && (
        <ProfileDialog
          mode={editor.mode}
          initialName={editor.mode === 'rename' ? editor.profile.name : ''}
          onCancel={() => setEditor(null)}
          onSubmit={async (name) => {
            await action(() => editor.mode === 'create' ? api.createProfile(name) : api.renameProfile(editor.profile.id, name));
            setEditor(null);
          }}
        />
      )}
      {deleteTarget && (
        <DeleteDialog
          profile={deleteTarget}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={async () => {
            await action(() => api.deleteProfile(deleteTarget.id));
            setDeleteTarget(null);
          }}
        />
      )}
    </main>
  );
}
