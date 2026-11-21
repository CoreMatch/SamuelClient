import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import DownloadQueue from './DownloadQueue';
import type { DownloadTask } from '../downloadQueue';

const activeTask: DownloadTask = {
  versionId: '1.21.4',
  name: 'client.jar',
  stage: 'jar',
  status: 'active',
  downloaded: 50,
  total: 100,
  percent: 50,
  error: '',
  updatedAt: 1,
};

describe('DownloadQueue', () => {
  it('shows progress and allows cancelling an active download', () => {
    const onCancel = vi.fn();
    render(<DownloadQueue tasks={[activeTask]} onCancel={onCancel} onDismiss={vi.fn()} />);

    expect(screen.getByLabelText('1.21.4: In progress')).toBeTruthy();
    expect(screen.getByText('Game file')).toBeTruthy();
    expect(screen.getByText('50%')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel download 1.21.4' }));

    expect(onCancel).toHaveBeenCalledWith('1.21.4');
  });

  it('allows dismissing a completed download', () => {
    const onDismiss = vi.fn();
    render(
      <DownloadQueue
        tasks={[{ ...activeTask, status: 'completed', stage: 'completed', percent: 100 }]}
        onCancel={vi.fn()}
        onDismiss={onDismiss}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss 1.21.4' }));
    expect(onDismiss).toHaveBeenCalledWith('1.21.4');
  });
});
