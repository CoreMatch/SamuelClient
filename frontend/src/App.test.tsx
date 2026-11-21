import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import App from './App';

const eventListeners = vi.hoisted(() => new Map<string, (payload: unknown) => void>());

vi.mock('@ha/ui', () => ({
  Asset: () => null,
}));

vi.mock('./components/HomeView', () => ({ default: () => 'Home page' }));
vi.mock('./components/VersionManager', () => ({ default: () => 'Versions page' }));
vi.mock('./components/AccountManager', () => ({ default: () => 'Accounts page' }));
vi.mock('./components/SettingsView', () => ({ default: () => 'Settings page' }));
vi.mock('./components/HandoffBanner', () => ({ default: () => null }));
vi.mock('./components/LoginView', () => ({ default: () => 'Login page' }));

vi.mock('./accountStorage', () => ({
  saveAccount: vi.fn(),
  restoreLastUsedAccount: vi.fn(async () => ({
    account: null,
    needsReLogin: false,
    keychainError: '',
  })),
  consumeOfflineRemovedNotice: vi.fn(() => null),
}));

vi.mock('../wailsjs/go/main/App', () => ({
  GetPendingHandoff: vi.fn(async () => ({ success: true, data: { status: 'none' } })),
  DismissHandoff: vi.fn(async () => ({ success: true })),
  GetInstalledVersions: vi.fn(async () => ({ success: true, data: { versions: [] } })),
  LaunchGame: vi.fn(async () => ({ success: true })),
  KeychainStatus: vi.fn(async () => ({
    success: true,
    data: { available: true, backend: 'keychain' },
  })),
  CancelInstall: vi.fn(async () => ({ success: true })),
}));

vi.mock('../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name: string, callback: (payload: unknown) => void) => {
    eventListeners.set(name, callback);
    return () => eventListeners.delete(name);
  }),
  EventsOff: vi.fn((name: string) => eventListeners.delete(name)),
}));

describe('App download queue', () => {
  beforeEach(() => {
    eventListeners.clear();
    localStorage.clear();
  });

  it('keeps live download progress visible when switching pages', async () => {
    render(<App />);
    expect(await screen.findByText('Home page')).toBeTruthy();
    const onProgress = eventListeners.get('download:progress');
    expect(onProgress).toBeTypeOf('function');

    act(() => {
      onProgress?.({
        version_id: '1.21.4',
        stage: 'asset',
        name: 'Minecraft assets',
        downloaded: 25,
        total: 100,
      });
    });
    expect(screen.getByLabelText('1.21.4: In progress')).toBeTruthy();
    expect(screen.getByText('25%')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Versions' }));
    expect(screen.getByText('Versions page')).toBeTruthy();
    expect(screen.getByLabelText('1.21.4: In progress')).toBeTruthy();
    expect(screen.getByText('25%')).toBeTruthy();
  });
});
