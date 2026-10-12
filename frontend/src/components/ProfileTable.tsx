import type { ProfileView } from '../types';
import { useLanguage } from '../i18n';

interface ProfileTableProps {
  profiles: ProfileView[];
  onLaunch(profile: ProfileView): void;
  onOpen(profile: ProfileView): void;
  onStop(profile: ProfileView): void;
  onRename(profile: ProfileView): void;
  onDelete(profile: ProfileView): void;
  onSelect(profile: ProfileView): void;
}

export function ProfileTable({ profiles, onLaunch, onOpen, onStop, onRename, onDelete, onSelect }: ProfileTableProps) {
  const { t, label } = useLanguage();
  if (profiles.length === 0) {
    return (
      <div className="empty-state" role="status">
        <h2>{t('No profiles')}</h2>
        <p>{t('Create a profile to start a dedicated browser runtime.')}</p>
      </div>
    );
  }

  return (
    <div className="table-shell">
      <table>
        <thead>
          <tr>
            <th>{t('Name')}</th>
            <th>{t('Browser')}</th>
            <th>{t('Network')}</th>
            <th>{t('Status')}</th>
            <th className="actions-heading">{t('Actions')}</th>
          </tr>
        </thead>
        <tbody>
          {profiles.map((profile) => (
            <tr key={profile.id} onClick={() => onSelect(profile)}>
              <td className="profile-name"><button className="profile-link" onClick={(event) => { event.stopPropagation(); onSelect(profile); }}>{profile.name}</button></td>
              <td>{profile.browser}</td>
              <td>{label(profile.network)}</td>
              <td><span className={`status status-${profile.status.toLowerCase()}`}>{label(profile.status)}</span></td>
              <td onClick={(event) => event.stopPropagation()}>
                <div className="row-actions">
                  {profile.status === 'Running' ? (
                    <>
                      <button className="button secondary small" onClick={() => onOpen(profile)}>{t('Open')}</button>
                      <button className="button secondary small" onClick={() => onStop(profile)}>{t('Stop')}</button>
                    </>
                  ) : (
                    <div className="launch-action">
                      <button className="button primary small" disabled={!profile.launch_ready} onClick={() => onLaunch(profile)} aria-describedby={!profile.launch_ready ? `blocked-${profile.id}` : undefined}>{t('Launch')}</button>
                      {!profile.launch_ready && <span className="launch-reason" id={`blocked-${profile.id}`}>{t(profile.launch_blocked_reason || 'Test network before launch')}</span>}
                    </div>
                  )}
                  <details className="more-menu">
                    <summary aria-label={t('More actions for {name}', { name: profile.name })}>{t('More')}</summary>
                    <div className="menu-panel">
                      <button type="button" onClick={() => onRename(profile)}>{t('Rename')}</button>
                      <button type="button" disabled={profile.status !== 'Stopped'} onClick={() => onDelete(profile)}>{t('Delete')}</button>
                    </div>
                  </details>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
