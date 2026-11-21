import { versionKey, type InstalledVersion } from './types';

const STORAGE_KEY = 'version_nicknames';

type Nicknames = Partial<Record<string, string>>;

export function readVersionNicknames(): Nicknames {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
    if (stored && typeof stored === 'object' && !Array.isArray(stored)) {
      return Object.fromEntries(Object.entries(stored).filter(([, value]) => typeof value === 'string'));
    }
  } catch {
    // A missing or unreadable preference should not hide installed versions.
  }
  return {};
}

export function saveVersionNickname(version: InstalledVersion, nickname: string): Nicknames {
  const nicknames = readVersionNicknames();
  const key = versionKey(version);
  if (nickname.trim()) {
    nicknames[key] = nickname.trim();
  } else {
    delete nicknames[key];
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(nicknames));
  return nicknames;
}
