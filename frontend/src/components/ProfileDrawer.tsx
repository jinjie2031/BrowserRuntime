import { useEffect, useRef, useState } from 'react';
import type { ProfileAPI, ProfileView } from '../types';
import { NetworkPanel } from './NetworkPanel';
import { useLanguage } from '../i18n';

interface Props {
  profile: ProfileView;
  api: ProfileAPI;
  onClose(): void;
  onChanged(): Promise<void>;
}

export function ProfileDrawer({ profile, api, onClose, onChanged }: Props) {
  const { t, label } = useLanguage();
  const [tab, setTab] = useState<'overview' | 'network'>('overview');
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (dialog.showModal) dialog.showModal();
    else dialog.setAttribute('open', '');
  }, []);

  return (
    <dialog ref={ref} className="drawer-dialog" aria-labelledby="drawer-title" onCancel={(event) => { event.preventDefault(); onClose(); }}>
      <header className="drawer-header">
        <div><p className="eyebrow">{t('Profile')}</p><h2 id="drawer-title">{profile.name}</h2></div>
        <button className="icon-button" aria-label={t('Close profile')} onClick={onClose} autoFocus>×</button>
      </header>
      <nav className="drawer-tabs" role="tablist" aria-label={t('Profile sections')}>
        {(['overview', 'network'] as const).map((name) => (
          <button key={name} id={`tab-${name}`} role="tab" aria-controls={`panel-${name}`} aria-selected={tab === name} tabIndex={tab === name ? 0 : -1} onClick={() => setTab(name)} onKeyDown={(event) => {
            if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
              const next = name === 'overview' ? 'network' : 'overview';
              setTab(next);
              ref.current?.querySelector<HTMLButtonElement>(`#tab-${next}`)?.focus();
              event.preventDefault();
            }
          }}>{t(name === 'overview' ? 'Overview' : 'Network')}</button>
        ))}
      </nav>
      <div className="drawer-body">
        {tab === 'overview' ? (
          <section id="panel-overview" role="tabpanel" aria-labelledby="tab-overview">
            <dl className="detail-list">
              <div><dt>{t('Name')}</dt><dd>{profile.name}</dd></div>
              <div><dt>{t('Browser')}</dt><dd>{profile.browser}</dd></div>
              <div><dt>{t('Status')}</dt><dd>{label(profile.status)}</dd></div>
              <div><dt>{t('Network Summary')}</dt><dd>{label(profile.network)}</dd></div>
              <div><dt>{t('Browser Identity')}</dt><dd>{t('Stable')}</dd></div>
            </dl>
          </section>
        ) : (
          <section id="panel-network" role="tabpanel" aria-labelledby="tab-network">
            <NetworkPanel profileID={profile.id} api={api} running={profile.status !== 'Stopped'} blockedReason={['ExternalRunning', 'InUse', 'Unknown'].includes(profile.status) ? profile.launch_blocked_reason : undefined} onChanged={onChanged} />
          </section>
        )}
      </div>
    </dialog>
  );
}
