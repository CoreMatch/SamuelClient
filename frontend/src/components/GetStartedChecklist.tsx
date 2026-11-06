import { CircleCheck, Circle } from 'lucide-react';
import { t } from '../i18n';

// GetStartedChecklist is the Modrinth-style first-run checklist (research
// 04 §A6): three facts derived from local state — an account exists, a
// version is installed, the game has launched once. It disappears when all
// three are done and never blocks anything.

interface GetStartedChecklistProps {
  hasAccount: boolean;
  installedCount: number;
  hasLaunched: boolean;
}

export default function GetStartedChecklist({ hasAccount, installedCount, hasLaunched }: GetStartedChecklistProps) {
  const items = [
    { done: hasAccount, label: t('checklist.signIn'), hint: t('checklist.signInHint') },
    { done: installedCount > 0, label: t('checklist.installVersion'), hint: t('checklist.installVersionHint') },
    { done: hasLaunched, label: t('checklist.firstLaunch'), hint: t('checklist.firstLaunchHint') },
  ];

  if (items.every(item => item.done)) {
    return null;
  }

  return (
    <section className="rounded-lg border border-edge bg-panel p-6">
      <h2 className="text-base font-semibold text-ink">{t('checklist.title')}</h2>
      <ul className="mt-4 flex flex-col gap-3">
        {items.map(item => (
          <li key={item.label} className="flex items-start gap-3">
            {item.done ? (
              <CircleCheck className="mt-0.5 size-4 shrink-0 text-ok" aria-hidden />
            ) : (
              <Circle className="mt-0.5 size-4 shrink-0 text-ink-faint" aria-hidden />
            )}
            <div>
              <p
                className={`text-sm ${item.done ? 'text-ink-faint line-through' : 'font-medium text-ink'}`}
              >
                {item.label}
              </p>
              {!item.done && <p className="mt-0.5 text-xs text-ink-muted">{item.hint}</p>}
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
