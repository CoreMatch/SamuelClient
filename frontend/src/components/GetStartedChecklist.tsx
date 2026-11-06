import { Asset, type AssetId } from '@ha/ui';
import { t } from '../i18n';

// GetStartedChecklist is the Modrinth-style first-run checklist (research
// 04 §A6), reduced to icon + single label + done graphic: three facts
// derived from local state, disappearing once all are done.

interface GetStartedChecklistProps {
  hasAccount: boolean;
  installedCount: number;
  hasLaunched: boolean;
}

export default function GetStartedChecklist({ hasAccount, installedCount, hasLaunched }: GetStartedChecklistProps) {
  const items: { done: boolean; label: string; icon: AssetId }[] = [
    { done: hasAccount, label: t('checklist.signIn'), icon: 'icon-login' },
    { done: installedCount > 0, label: t('checklist.installVersion'), icon: 'icon-download' },
    { done: hasLaunched, label: t('checklist.firstLaunch'), icon: 'icon-launch' },
  ];

  if (items.every(item => item.done)) {
    return null;
  }

  return (
    <section className="rounded-lg bg-panel p-8">
      <h2 className="text-xl font-semibold text-ink">{t('checklist.title')}</h2>
      <ul className="mt-5 flex flex-col gap-4">
        {items.map(({ done, label, icon }) => (
          <li key={label} className="flex items-center gap-4">
            <span
              className={`flex size-10 shrink-0 items-center justify-center rounded-full ${
                done ? 'bg-ok/15 text-ok' : 'bg-white/6 text-ink-faint'
              }`}
            >
              <Asset id={done ? 'icon-check' : icon} size={20} />
            </span>
            <span className={`text-base ${done ? 'text-ink-faint line-through' : 'font-medium text-ink'}`}>
              {label}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
