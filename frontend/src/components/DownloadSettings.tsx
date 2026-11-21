import { useEffect, useId, useState } from 'react';
import { ChevronRight } from 'lucide-react';
import { GetDownloadSettings, SetDownloadSettings } from '../../wailsjs/go/main/App';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Callout } from '../ui/callout';
import { Field, NumberField, TextField } from '../ui/field';

interface Preferences {
  source: 'bmclapi' | 'official' | 'custom';
  custom_shape: 'bmclapi' | 'official';
  custom_url: string;
  concurrency: number;
}

const selectClasses =
  'h-11 w-full rounded-md border border-edge bg-raised px-4 text-base text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong';

export default function DownloadSettings() {
  const id = useId();
  const [settings, setSettings] = useState<Preferences | null>(null);
  const [concurrency, setConcurrency] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const result = await GetDownloadSettings();
        if (!active) return;
        if (!result.success || !result.data) {
          setError(result.error || t('settings.downloadLoadFailed'));
          return;
        }
        setSettings(result.data);
        setConcurrency(String(result.data.concurrency));
      } catch {
        if (active) setError(t('settings.downloadLoadFailed'));
      }
    };
    void load();
    return () => { active = false; };
  }, []);

  const save = async () => {
    if (!settings || saving) return;
    setSaving(true);
    setError('');
    setSaved(false);
    try {
      const result = await SetDownloadSettings(settings.source, settings.custom_shape, settings.custom_url, Number(concurrency));
      if (result.success && result.data) {
        setSettings(result.data);
        setConcurrency(String(result.data.concurrency));
        setSaved(true);
      } else {
        setError(result.error || t('settings.failedSave'));
      }
    } catch {
      setError(t('settings.failedSave'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <details className="group rounded-md border border-edge">
      <summary className="flex cursor-pointer list-none items-center gap-2 rounded-md p-4 font-medium text-ink focus-visible:outline-2 [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-4 group-open:rotate-90" aria-hidden />
        {t('settings.download')}
      </summary>
      {settings && (
        <form
          className="px-4 pb-4"
          onChange={() => { setSaved(false); setError(''); }}
          onSubmit={event => { event.preventDefault(); void save(); }}
        >
          <fieldset disabled={saving} className="flex flex-col gap-4">
            <Field label={t('settings.downloadSource')} htmlFor={`${id}-source`}>
              <select id={`${id}-source`} className={selectClasses} value={settings.source}
                onChange={e => setSettings({ ...settings, source: e.target.value as Preferences['source'] })}>
                <option value="bmclapi">{t('settings.downloadMirror')}</option>
                <option value="official">{t('settings.downloadOfficial')}</option>
                <option value="custom">{t('settings.downloadCustom')}</option>
              </select>
            </Field>
            {settings.source === 'custom' && (
              <>
                <Field label={t('settings.downloadShape')} htmlFor={`${id}-shape`}>
                  <select id={`${id}-shape`} className={selectClasses} value={settings.custom_shape}
                    onChange={e => setSettings({ ...settings, custom_shape: e.target.value as Preferences['custom_shape'] })}>
                    <option value="bmclapi">{t('settings.downloadMirrorShape')}</option>
                    <option value="official">{t('settings.downloadOfficialShape')}</option>
                  </select>
                </Field>
                <TextField label={t('settings.downloadURL')} type="url" required
                  placeholder="https://mirror.example.com" value={settings.custom_url}
                  aria-describedby={`${id}-source-hint`}
                  onChange={e => setSettings({ ...settings, custom_url: e.target.value })} />
                <p id={`${id}-source-hint`} className="text-sm text-ink-muted">
                  {t(settings.custom_shape === 'official' ? 'settings.downloadOfficialHint' : 'settings.downloadMirrorHint')}
                  {' '}{t('settings.downloadTrustHint')}
                </p>
              </>
            )}
            <NumberField label={t('settings.downloadConcurrency')} min={1} max={32} step={1} required
              value={concurrency} onChange={e => setConcurrency(e.target.value)}
              aria-describedby={`${id}-concurrency-hint`} />
            <p id={`${id}-concurrency-hint`} className="text-sm text-ink-muted">{t('settings.downloadConcurrencyHint')}</p>
            <Button type="submit" size="sm" loading={saving} className="self-start">{t('settings.downloadSave')}</Button>
            {saved && <p role="status" className="text-sm text-ink-muted">{t('settings.downloadSaved')}</p>}
          </fieldset>
        </form>
      )}
      {error && <div className="px-4 pb-4"><Callout tone="danger">{error}</Callout></div>}
      {!settings && !error && <p className="px-4 pb-4 text-sm text-ink-muted">{t('settings.downloadLoading')}</p>}
    </details>
  );
}
