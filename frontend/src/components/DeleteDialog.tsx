import type { ProfileView } from '../types';
import { useLanguage } from '../i18n';

interface DeleteDialogProps {
  profile: ProfileView;
  onCancel(): void;
  onConfirm(): Promise<void>;
}

export function DeleteDialog({ profile, onCancel, onConfirm }: DeleteDialogProps) {
  const { t } = useLanguage();
  return (
    <div className="modal-backdrop" role="presentation">
      <section className="modal compact" role="alertdialog" aria-modal="true" aria-labelledby="delete-title" aria-describedby="delete-description">
        <header className="modal-header">
          <h2 id="delete-title">{t('Delete Profile?')}</h2>
          <button type="button" className="icon-button" aria-label={t('Close')} onClick={onCancel}>×</button>
        </header>
        <div className="modal-body">
          <p id="delete-description">{t('Delete ')}<strong>{profile.name}</strong>{t(' and its browser data? This cannot be undone.')}</p>
        </div>
        <footer className="modal-footer">
          <button type="button" className="button secondary" onClick={onCancel}>{t('Cancel')}</button>
          <button type="button" className="button danger" onClick={onConfirm}>{t('Delete')}</button>
        </footer>
      </section>
    </div>
  );
}
