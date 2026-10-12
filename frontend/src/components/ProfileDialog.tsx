import { FormEvent, useEffect, useRef, useState } from 'react';
import { useLanguage } from '../i18n';

interface ProfileDialogProps {
  mode: 'create' | 'rename';
  initialName?: string;
  onCancel(): void;
  onSubmit(name: string): Promise<void>;
}

export function ProfileDialog({ mode, initialName = '', onCancel, onSubmit }: ProfileDialogProps) {
  const { t } = useLanguage();
  const [name, setName] = useState(initialName);
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => inputRef.current?.focus(), []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!name.trim() || submitting) return;
    setSubmitting(true);
    try {
      await onSubmit(name);
    } finally {
      setSubmitting(false);
    }
  }

  const title = mode === 'create' ? 'New Profile' : 'Rename Profile';
  return (
    <div className="modal-backdrop" role="presentation">
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby="profile-dialog-title">
        <form onSubmit={submit}>
          <header className="modal-header">
            <h2 id="profile-dialog-title">{t(title)}</h2>
            <button type="button" className="icon-button" aria-label={t('Close')} onClick={onCancel}>×</button>
          </header>
          <div className="modal-body">
            <label htmlFor="profile-name">{t('Profile Name')}</label>
            <input
              ref={inputRef}
              id="profile-name"
              value={name}
              maxLength={80}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
            />
            <div className="field-row" aria-label={t('Browser')}>
              <span>{t('Browser')}</span>
              <strong>CloakBrowser</strong>
            </div>
          </div>
          <footer className="modal-footer">
            <button type="button" className="button secondary" onClick={onCancel}>{t('Cancel')}</button>
            <button type="submit" className="button primary" disabled={!name.trim() || submitting}>
              {t(submitting ? 'Saving…' : mode === 'create' ? 'Create' : 'Save')}
            </button>
          </footer>
        </form>
      </section>
    </div>
  );
}
