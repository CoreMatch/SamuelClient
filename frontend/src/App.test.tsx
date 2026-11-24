import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import App from './App';
import { GetUpdateStatus } from '../wailsjs/go/main/App';

const eventListeners = vi.hoisted(() => new Map<string, (payload: unknown) => void>());

vi.mock('@ha/ui', () => ({
  Asset: () => null,
}));

vi.mock('./components/HomeView', () => ({ default: () => 'Home page' }));
vi.mock('./components/VersionManager', () => ({ default: () => 'Versions page' }));
vi.mock('./components/ProfileView', () => ({ default: () => 'Profile page' }));
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
  GetUpdateStatus: vi.fn(async () => ({
    success: true,
    data: {
      state: 'current', version: '', notes: '', mandatory: false, allowDowngrade: false,
      downloaded: 0, total: 0, error: '',
    },
  })),
  CheckForUpdates: vi.fn(async () => ({ success: true })),
  InstallUpdate: vi.fn(async () => ({ success: true })),
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
    vi.mocked(GetUpdateStatus).mockResolvedValue({
      success: true,
      data: {
        state: 'current', version: '', notes: '', mandatory: false, allowDowngrade: false,
        downloaded: 0, total: 0, error: '',
      },
    });
  });

  it('keeps live download progress visible when switching pages', async () => {
    render(<App />);
    expect(await screen.findByText('Home page')).toBeTruthy();
    expect(screen.queryByRole('region', { name: 'Download queue' })).toBeNull();
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

  it('blocks the launcher behind a mandatory update', async () => {
    vi.mocked(GetUpdateStatus).mockResolvedValue({
      success: true,
      data: {
        state: 'available', version: 'v1.2.0', notes: 'Security fix', mandatory: true,
        allowDowngrade: false, downloaded: 0, total: 0, error: '',
      },
    });

    render(<App />);

    expect(await screen.findByText('Required update')).toBeTruthy();
    expect(screen.queryByText('Home page')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Install now' })).toBeTruthy();
  });

  it('opens the Profile view from primary navigation', async () => {
    render(<App />);
    fireEvent.click(await screen.findByRole('button', { name: 'Profile' }));

    expect(screen.getByText('Profile page')).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Profile' })).toBeTruthy();
  });
});
