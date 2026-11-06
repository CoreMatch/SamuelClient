import { describe, it, expect } from 'vitest';
import { t, setLocale } from './i18n';

describe('i18n', () => {
  it('resolves English catalog entries', () => {
    expect(t('handoff.title')).toBe('Match found');
  });

  it('substitutes placeholders', () => {
    expect(t('handoff.server', { host: 'h.local', port: 25565 })).toBe('Server: h.local:25565');
  });

  it('falls back to English for unknown locales', () => {
    setLocale('xx');
    expect(t('handoff.joinNow')).toBe('Join now');
    setLocale('en');
  });
});
