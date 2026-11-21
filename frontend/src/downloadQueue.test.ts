import { describe, expect, it } from 'vitest';
import { updateDownloadQueue, type DownloadTask } from './downloadQueue';

describe('updateDownloadQueue', () => {
  it('tracks concurrent downloads and their byte progress', () => {
    const tasks = updateDownloadQueue([], {
      version_id: '1.21.4',
      stage: 'asset',
      name: 'assets',
      downloaded: 25,
      total: 100,
    }, 1);
    const concurrent = updateDownloadQueue(tasks, {
      version_id: 'java-25',
      stage: 'java-download',
      name: 'Temurin Java 25',
      downloaded: 50,
      total: 100,
    }, 2);

    expect(concurrent).toHaveLength(2);
    expect(concurrent[0]).toMatchObject({ versionId: 'java-25', status: 'active', percent: 50 });
    expect(concurrent[1]).toMatchObject({ versionId: '1.21.4', status: 'active', percent: 25 });
  });

  it('retains a completed task and removes its active progress state', () => {
    const active = updateDownloadQueue([], {
      version_id: '1.21.4',
      stage: 'jar',
      downloaded: 20,
      total: 100,
    }, 1);
    const completed = updateDownloadQueue(active, {
      version_id: '1.21.4',
      stage: 'completed',
    }, 2);

    expect(completed).toHaveLength(1);
    expect(completed[0]).toMatchObject({ status: 'completed', percent: 100 });
  });

  it('retains failure details and cancelled tasks for the queue', () => {
    const failed = updateDownloadQueue([], {
      version_id: 'java-21',
      stage: 'failed',
      error: 'checksum mismatch',
    }, 1);
    const cancelled = updateDownloadQueue(failed, {
      version_id: '1.20.1',
      stage: 'cancelled',
    }, 2);

    expect(cancelled.map(task => task.status)).toEqual(['cancelled', 'failed']);
    expect(cancelled[1].error).toBe('checksum mismatch');
  });

  it('keeps all active tasks while limiting retained history', () => {
    let tasks: DownloadTask[] = [];
    for (let i = 0; i < 8; i++) {
      tasks = updateDownloadQueue(tasks, {
        version_id: `done-${i}`,
        stage: 'completed',
      }, i);
    }
    for (let i = 0; i < 7; i++) {
      tasks = updateDownloadQueue(tasks, {
        version_id: `active-${i}`,
        stage: 'asset',
      }, 20 + i);
    }

    expect(tasks.filter(task => task.status === 'active')).toHaveLength(7);
    expect(tasks.filter(task => task.status !== 'active')).toHaveLength(5);
  });
});
