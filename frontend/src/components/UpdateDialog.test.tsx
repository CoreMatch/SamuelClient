import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { UpdateDialog } from './UpdateDialog';
import type { UpdateStatus } from '../useUpdateStatus';

const quit = vi.hoisted(() => vi.fn());
vi.mock('../../wailsjs/runtime/runtime', () => ({ Quit: quit }));

const available: UpdateStatus = {
  state: 'available',
  version: 'v0.3.2',
  notes: 'Fixes and improvements.',
  mandatory: false,
  allowDowngrade: false,
  downloaded: 0,
  total: 100,
  error: '',
};

function renderDialog(status: UpdateStatus, handlers = {}) {
  const props = {
    status,
    open: true,
    onInstall: vi.fn(),
    onDismiss: vi.fn(),
    onRetry: vi.fn(),
    ...handlers,
  };
  render(<UpdateDialog {...props} />);
  return props;
}

describe('UpdateDialog', () => {
  it('asks for confirmation before installation', () => {
    const props = renderDialog(available);
    expect(screen.getByText('Version v0.3.2')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Install now' }));
    expect(props.onInstall).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole('button', { name: 'Later' }));
    expect(props.onDismiss).toHaveBeenCalledOnce();
  });

  it('does not offer dismissal for mandatory updates', () => {
    renderDialog({ ...available, mandatory: true });
    expect(screen.getByText('Required update')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Later' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Dismiss update' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Exit' }));
    expect(quit).toHaveBeenCalledOnce();
  });

  it('shows download progress without allowing dismissal', () => {
    renderDialog({ ...available, state: 'downloading', downloaded: 25 });
    expect(screen.getByText('Downloading update')).toBeTruthy();
    expect(screen.getByText('25%')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Later' })).toBeNull();
  });

  it('shows a retry action after failure', () => {
    const props = renderDialog({ ...available, state: 'error', error: 'Network unavailable' });
    expect(screen.getByRole('alert').textContent).toContain('Network unavailable');
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(props.onRetry).toHaveBeenCalledOnce();
  });
});
