import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import { LogOut, Upload, UserRound } from 'lucide-react';
import type { StoredAccount } from '../accountStorage';
import { t } from '../i18n';
import { AlertDialog } from '../ui/dialog';
import { Button } from '../ui/button';
import { Badge } from '../ui/badge';
import { Callout } from '../ui/callout';
import { LoadingSkeleton } from './states';

const SkinViewer3D = lazy(() => import('./SkinViewer3D'));
import {
  DeleteProfileTexture,
  GetProfileTextures,
  UploadProfileTexture,
} from '../../wailsjs/go/main/App';

type TextureType = 'skin' | 'cape';

interface ProfileTexture {
  url?: string;
  hash?: string;
}

interface PlayerProfile {
  id: string;
  name: string;
  textures?: Partial<Record<TextureType, ProfileTexture>>;
}

interface ProfileViewProps {
  account: StoredAccount | null;
  onSignIn: () => void;
  onSignOut: () => Promise<void>;
}

function fileToBase64(file: File): Promise<string> {
  return file.arrayBuffer().then(buffer => {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    const chunkSize = 0x8000;
    for (let offset = 0; offset < bytes.length; offset += chunkSize) {
      binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
    }
    return btoa(binary);
  });
}

export default function ProfileView({ account, onSignIn, onSignOut }: ProfileViewProps) {
  // StoredAccount.accessToken is the HA Yggdrasil session for both login
  // paths. The MSA OAuth refresh token is never exposed to this component.
  const yggdrasilAccessToken = account?.accessToken ?? '';
  const playerUUID = account?.uuid ?? '';
  const [profile, setProfile] = useState<PlayerProfile | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingError, setLoadingError] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [files, setFiles] = useState<Partial<Record<TextureType, File>>>({});
  const [previews, setPreviews] = useState<Partial<Record<TextureType, string>>>({});
  const [busy, setBusy] = useState<TextureType | 'signout' | null>(null);
  const [confirmAction, setConfirmAction] = useState<TextureType | 'signout' | null>(null);
  const inputs = useRef<Partial<Record<TextureType, HTMLInputElement | null>>>({});

  useEffect(() => {
    let cancelled = false;
    if (!yggdrasilAccessToken) {
      setProfile(null);
      setLoading(false);
      setLoadingError('');
      return;
    }
    setLoading(true);
    setLoadingError('');
    GetProfileTextures(yggdrasilAccessToken)
      .then(result => {
        if (cancelled) return;
        if (!result.success || !result.data) {
          setLoadingError(result.error || t('profile.loadFailed'));
          return;
        }
        const data = result.data as { profiles?: PlayerProfile[] };
        const nextProfile = data.profiles?.find(item => item.id === playerUUID) ?? data.profiles?.[0] ?? null;
        setProfile(nextProfile);
      })
      .catch(() => {
        if (!cancelled) setLoadingError(t('profile.loadFailed'));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [yggdrasilAccessToken, playerUUID]);

  useEffect(() => {
    const urls: Partial<Record<TextureType, string>> = {};
    for (const type of ['skin', 'cape'] as const) {
      if (files[type]) urls[type] = URL.createObjectURL(files[type]);
    }
    setPreviews(urls);
    return () => Object.values(urls).forEach(URL.revokeObjectURL);
  }, [files]);

  const handleFile = (type: TextureType, file?: File) => {
    setError('');
    setNotice('');
    if (!file) return;
    if (file.type !== 'image/png') {
      setError(t('profile.pngOnly'));
      return;
    }
    if (file.size > 2 * 1024 * 1024) {
      setError(t('profile.fileTooLarge'));
      return;
    }
    setFiles(current => ({ ...current, [type]: file }));
  };

  const reloadProfile = async () => {
    if (!yggdrasilAccessToken) return;
    setLoading(true);
    try {
      const result = await GetProfileTextures(yggdrasilAccessToken);
      if (!result.success || !result.data) {
        setError(result.error || t('profile.loadFailed'));
        return;
      }
      const data = result.data as { profiles?: PlayerProfile[] };
      const nextProfile = data.profiles?.find(item => item.id === playerUUID) ?? data.profiles?.[0] ?? null;
      setProfile(nextProfile);
    } catch {
      setError(t('profile.loadFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleUpload = async (type: TextureType) => {
    const file = files[type];
    if (!yggdrasilAccessToken || !profile || !file) return;
    setBusy(type);
    setError('');
    setNotice('');
    try {
      const result = await UploadProfileTexture(
        yggdrasilAccessToken,
        profile.id,
        type,
        '',
        file.name,
        await fileToBase64(file),
      );
      if (!result.success) {
        setError(result.error || t('profile.uploadFailed'));
        return;
      }
      setFiles(current => ({ ...current, [type]: undefined }));
      const input = inputs.current[type];
      if (input) input.value = '';
      setNotice(t('profile.uploaded', { type: t(`profile.${type}`) }));
      await reloadProfile();
    } catch {
      setError(t('profile.uploadFailed'));
    } finally {
      setBusy(null);
    }
  };

  const handleConfirm = async () => {
    if (!confirmAction) return;
    const action = confirmAction;
    setConfirmAction(null);
    setBusy(action);
    setError('');
    setNotice('');
    try {
      if (action === 'signout') {
        await onSignOut();
        return;
      }
      if (!yggdrasilAccessToken || !profile) return;
      const result = await DeleteProfileTexture(yggdrasilAccessToken, profile.id, action);
      if (!result.success) {
        setError(result.error || t('profile.deleteFailed'));
        return;
      }
      setNotice(t('profile.deleted', { type: t(`profile.${action}`) }));
      setFiles(current => ({ ...current, [action]: undefined }));
      await reloadProfile();
    } catch {
      setError(t('profile.deleteFailed'));
    } finally {
      setBusy(null);
    }
  };

  if (!account) {
    return (
      <div className="rounded-lg bg-panel px-8 py-16 text-center">
        <UserRound className="mx-auto size-12 text-ink-faint" aria-hidden />
        <h2 className="mt-4 text-xl font-semibold text-ink">{t('profile.signedOutTitle')}</h2>
        <p className="mx-auto mt-2 max-w-md text-sm text-ink-muted">{t('profile.signedOutDescription')}</p>
        <Button className="mt-6" onClick={onSignIn}>{t('nav.signIn')}</Button>
      </div>
    );
  }

  const activeProfile = profile ?? { id: account.uuid, name: account.username };
  const previewSkin = previews.skin || profile?.textures?.skin?.url;
  const previewCape = previews.cape || profile?.textures?.cape?.url;
  const textureCard = (type: TextureType) => {
    const file = files[type];
    const uploading = busy === type && !!file;
    const deleting = busy === type && !file;
    return (
      <section key={type} className="rounded-lg border border-edge bg-raised p-5">
        <div className="mb-4 flex items-center justify-between gap-3">
          <h3 className="text-lg font-semibold text-ink">{t(`profile.${type}`)}</h3>
          <Badge tone={profile?.textures?.[type]?.url ? 'success' : 'neutral'}>
            {profile?.textures?.[type]?.url ? t('profile.active') : t('profile.notSet')}
          </Badge>
        </div>
        <input
          ref={element => { inputs.current[type] = element; }}
          type="file"
          accept="image/png"
          className="sr-only"
          aria-label={t('profile.chooseFile', { type: t(`profile.${type}`) })}
          onChange={event => handleFile(type, event.target.files?.[0])}
        />
        <div className="flex flex-wrap gap-2">
          <Button
            variant="secondary"
            disabled={busy !== null || loading}
            onClick={() => inputs.current[type]?.click()}
          >
            <Upload className="size-4" aria-hidden />
            {file ? file.name : t('profile.chooseFile', { type: t(`profile.${type}`) })}
          </Button>
          {file && (
            <Button disabled={busy !== null || loading} loading={uploading} onClick={() => void handleUpload(type)}>
              {t('profile.upload')}
            </Button>
          )}
          {profile?.textures?.[type]?.url && (
            <Button
              variant="danger"
              disabled={busy !== null || loading}
              loading={deleting}
              onClick={() => setConfirmAction(type)}
            >
              {t('profile.remove')}
            </Button>
          )}
        </div>
      </section>
    );
  };

  return (
    <div className="space-y-6">
      {error && <Callout tone="danger" onDismiss={() => setError('')}>{error}</Callout>}
      {loadingError && (
        <Callout tone="danger" onDismiss={() => setLoadingError('')}>
          {loadingError}
          <button type="button" className="ml-2 underline" onClick={() => void reloadProfile()}>{t('common.retry')}</button>
        </Callout>
      )}
      {notice && <Callout tone="success" onDismiss={() => setNotice('')}>{notice}</Callout>}

      <section className="rounded-lg bg-panel p-6">
        <div className="grid items-center gap-6 lg:grid-cols-[minmax(0,1fr)_300px]">
          <div>
            <div className="flex flex-wrap items-center gap-4">
              <div className="grid size-16 place-items-center rounded-full bg-raised text-accent">
                <UserRound className="size-8" aria-hidden />
              </div>
              <div className="min-w-0 flex-1">
                <h2 className="truncate text-2xl font-semibold text-ink">{activeProfile.name}</h2>
                <Badge tone="success" className="mt-2">{t(account.authType === 'microsoft' ? 'profile.microsoft' : 'profile.hrpauth')}</Badge>
              </div>
              <Button variant="ghost" disabled={busy !== null} onClick={() => setConfirmAction('signout')}>
                <LogOut className="size-4" aria-hidden />
                {t('profile.signOut')}
              </Button>
            </div>
            <dl className="mt-6 grid gap-4 border-t border-edge pt-5 sm:grid-cols-2">
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-ink-faint">{t('profile.playerName')}</dt>
                <dd className="mt-1 text-sm text-ink">{activeProfile.name}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-ink-faint">{t('profile.uuid')}</dt>
                <dd className="mt-1 break-all font-mono text-sm text-ink">{activeProfile.id}</dd>
              </div>
            </dl>
          </div>
          {previewSkin || previewCape ? (
            <Suspense fallback={(
              <div className="grid min-h-[300px] place-items-center rounded-md bg-raised text-sm text-ink-muted">
                {t('profile.previewLoading')}
              </div>
            )}>
              <SkinViewer3D
                skinUrl={previewSkin}
                capeUrl={previewCape}
              />
            </Suspense>
          ) : (
            <div className="grid min-h-[300px] place-items-center rounded-md bg-raised px-6 text-center text-sm text-ink-faint">
              {t('profile.noTexture')}
            </div>
          )}
        </div>
      </section>

      <div>
        <div className="mb-4">
          <h2 className="text-xl font-semibold text-ink">{t('profile.texturesTitle')}</h2>
          <p className="mt-1 text-sm text-ink-muted">{t('profile.texturesDescription')}</p>
        </div>
        {loading ? <LoadingSkeleton rows={3} /> : (
          <div className="grid gap-4 lg:grid-cols-2">
            {textureCard('skin')}
            {textureCard('cape')}
          </div>
        )}
      </div>

      <AlertDialog
        open={confirmAction !== null}
        onOpenChange={open => { if (!open && busy === null) setConfirmAction(null); }}
        title={confirmAction === 'signout' ? t('profile.signOutTitle') : t('profile.removeTitle')}
        description={confirmAction === 'signout'
          ? t('profile.signOutConfirm')
          : t('profile.removeConfirm', { type: confirmAction ? t(`profile.${confirmAction}`) : '' })}
        confirmLabel={confirmAction === 'signout' ? t('profile.signOut') : t('profile.remove')}
        cancelLabel={t('common.cancel')}
        onConfirm={() => void handleConfirm()}
      />
    </div>
  );
}
