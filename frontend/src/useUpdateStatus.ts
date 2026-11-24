import { useCallback, useEffect, useState } from 'react';
import { CheckForUpdates, GetUpdateStatus, InstallUpdate } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

export interface UpdateStatus {
  state: 'idle' | 'checking' | 'current' | 'available' | 'downloading' | 'ready' | 'installing' | 'error';
  version: string;
  notes: string;
  mandatory: boolean;
  allowDowngrade: boolean;
  downloaded: number;
  total: number;
  error: string;
}

const idleStatus: UpdateStatus = {
  state: 'idle',
  version: '',
  notes: '',
  mandatory: false,
  allowDowngrade: false,
  downloaded: 0,
  total: 0,
  error: '',
};

export function useUpdateStatus() {
  const [status, setStatus] = useState<UpdateStatus>(idleStatus);
  const [dismissedVersion, setDismissedVersion] = useState('');

  useEffect(() => {
    let active = true;
    GetUpdateStatus()
      .then(result => {
        if (active && result.success && result.data) setStatus(result.data as UpdateStatus);
      })
      .catch(() => {});
    const unsubscribe = EventsOn('launcher:update', (payload: UpdateStatus) => {
      if (active && payload) setStatus(payload);
    });
    return () => {
      active = false;
      unsubscribe();
    };
  }, []);

  const dismiss = useCallback(() => {
    if (!status.mandatory) setDismissedVersion(status.version);
  }, [status.mandatory, status.version]);

  const install = useCallback(async () => {
    const result = await InstallUpdate().catch(() => ({ success: false, error: 'update request failed' }));
    if (!result.success) {
      setStatus(current => ({ ...current, state: 'error', error: result.error || 'update request failed' }));
      return;
    }
    setStatus(current => ({ ...current, state: 'downloading', error: '' }));
  }, []);

  const retry = useCallback(async () => {
    if (status.version) {
      await install();
      return;
    }
    await CheckForUpdates().catch(() => {});
  }, [install, status.version]);

  const dismissed = status.state === 'available' && !status.mandatory && dismissedVersion === status.version;
  const visible = Boolean(status.version) && status.state !== 'current' && !dismissed;
  const blocking = Boolean(status.version) && status.mandatory && status.state !== 'current';

  return { status, visible, blocking, dismiss, install, retry };
}
