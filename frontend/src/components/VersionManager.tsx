import { useState, useEffect, useCallback, useRef } from 'react';
import { Pencil, Trash2 } from 'lucide-react';
import { Asset } from '@ha/ui';
import { GetInstalledVersions } from '../../wailsjs/go/main/App';
import { installedFromDisk, versionKey, type DiskVersion, type InstalledVersion } from '../types';
import InstallWizard from './InstallWizard';
import { EmptyState, ErrorState, LoadingSkeleton } from './states';
import { t } from '../i18n';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs';
import { Badge } from '../ui/badge';
import { Button, IconButton } from '../ui/button';
import { TextField } from '../ui/field';
import { readVersionNicknames, saveVersionNickname } from '../versionNicknames';

interface VersionManagerProps {
  onSelectVersion: (version: InstalledVersion) => void;
}

const STORAGE_KEY = 'installed_versions';

export default function VersionManager({ onSelectVersion }: VersionManagerProps) {
  const [localVersions, setLocalVersions] = useState<InstalledVersion[]>([]);
  const [diskVersions, setDiskVersions] = useState<InstalledVersion[]>([]);
  const [loading, setLoading] = useState(false);
  const [scanFailed, setScanFailed] = useState(false);
  const [tabValue, setTabValue] = useState('install');
  const [nicknames, setNicknames] = useState(readVersionNicknames);
  const [nicknameEdit, setNicknameEdit] = useState<{ key: string; draft: string; failed: boolean } | null>(null);
  const editButton = useRef<HTMLButtonElement | null>(null);

  const closeNicknameEdit = () => {
    setNicknameEdit(null);
    editButton.current?.focus();
  };

  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const versions = JSON.parse(stored);
        if (Array.isArray(versions)) setLocalVersions(versions);
      }
    } catch (err) {
      console.error('Failed to load installed versions:', err);
    }
  }, []);

  const scanInstalledVersions = useCallback(async () => {
    setLoading(true);
    setScanFailed(false);
    try {
      const result = await GetInstalledVersions();
      if (!result.success || !Array.isArray(result.data?.versions)) {
        setScanFailed(true);
        return;
      }
      setDiskVersions(installedFromDisk(result.data.versions as DiskVersion[]));
    } catch {
      setScanFailed(true);
    } finally {
      setLoading(false);
    }
  }, []);

  const diskIds = new Set(diskVersions.map(version => version.id));
  const installedVersions = [...localVersions.filter(v => !v.gameDir && !diskIds.has(v.id)), ...diskVersions];

  const handleDelete = (versionId: string) => {
    const updated = localVersions.filter(v => v.id !== versionId);
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
    } catch (err) {
      console.error('Failed to save installed versions:', err);
    }
    setLocalVersions(updated);
  };

  // Version-type badge tones come from the shared semantic palette.
  const getVersionTypeTone = (type: string) => {
    switch (type) {
      case 'release':
        return 'success' as const;
      case 'snapshot':
        return 'warning' as const;
      case 'old_beta':
        return 'info' as const;
      default:
        return 'neutral' as const;
    }
  };

  return (
    <Tabs value={tabValue} onValueChange={value => {
      setTabValue(value);
      setNicknameEdit(null);
      if (value === 'installed') void scanInstalledVersions();
    }}>
      <TabsList>
        <TabsTrigger value="install">{t('versions.tab.install')}</TabsTrigger>
        <TabsTrigger value="installed">{t('versions.tab.installed')}</TabsTrigger>
      </TabsList>

      <TabsContent value="install">
        <InstallWizard onInstalled={scanInstalledVersions} />
      </TabsContent>

      <TabsContent value="installed">
        {loading && installedVersions.length === 0 ? <LoadingSkeleton /> : scanFailed && installedVersions.length === 0 ? (
          <ErrorState data={t('versions.listData')} onRetry={scanInstalledVersions} />
        ) : installedVersions.length === 0 ? (
          <EmptyState
            asset="state-no-installed-version"
            title={t('versions.emptyTitle')}
            actionLabel={t('versions.browse')}
            onAction={() => setTabValue('install')}
          />
        ) : (
          <div className="rounded-lg bg-panel">
            {installedVersions.map((version, index) => {
              const key = versionKey(version);
              const nickname = nicknames[key];
              const edit = nicknameEdit?.key === key ? nicknameEdit : null;
              const versionLabel = version.gameDir ? `${version.id} (${version.gameDir})` : version.id;
              return (
                <div key={key} className={index > 0 ? 'border-t border-edge' : ''}>
                  <div className="flex items-center gap-3 px-5 hover:bg-white/4">
                    <button
                      type="button"
                      aria-label={t('versions.selectLabel', { version: nickname ? `${nickname}, ${versionLabel}` : versionLabel })}
                      onClick={() => onSelectVersion(version)}
                      className="min-w-0 flex-1 cursor-pointer rounded-md py-5 text-left focus-visible:outline-2 focus-visible:outline-edge-strong"
                    >
                      <span className="flex items-center gap-2.5">
                        <span className="min-w-0 wrap-anywhere text-base font-medium text-ink">{nickname || version.id}</span>
                        <span aria-hidden="true" className="shrink-0">
                          <Asset id="icon-check" size={20} className="text-ok" />
                        </span>
                      </span>
                      {nickname && <span className="mt-1 block wrap-anywhere text-sm text-ink-muted">{version.id}</span>}
                      {version.gameDir && <span className="mt-1 block break-all text-sm text-ink-muted">{version.gameDir}</span>}
                      <span className="mt-1.5 flex items-center gap-2">
                        <Badge tone={getVersionTypeTone(version.type)}>{version.type}</Badge>
                        {version.loaderType && (
                          <Badge>{`${version.loaderType} ${version.loaderVersion || ''}`}</Badge>
                        )}
                      </span>
                    </button>
                    <IconButton
                      className="shrink-0"
                      label={t('versions.editNickname', { version: versionLabel })}
                      aria-expanded={!!edit}
                      onClick={e => {
                        if (edit) {
                          closeNicknameEdit();
                        } else {
                          editButton.current = e.currentTarget;
                          setNicknameEdit({ key, draft: nickname ?? '', failed: false });
                        }
                      }}
                    >
                      <Pencil className="size-5" aria-hidden />
                    </IconButton>
                    {!diskIds.has(version.id) && (
                      <IconButton
                        className="shrink-0"
                        label={t('versions.deleteLabel', { version: version.id })}
                        tone="danger"
                        onClick={() => {
                          if (edit) setNicknameEdit(null);
                          handleDelete(version.id);
                        }}
                      >
                        <Trash2 className="size-5" aria-hidden />
                      </IconButton>
                    )}
                  </div>
                  {edit && (
                    <form
                      className="space-y-3 px-5 pb-5"
                      onSubmit={e => {
                        e.preventDefault();
                        try {
                          setNicknames(saveVersionNickname(version, edit.draft));
                          closeNicknameEdit();
                        } catch {
                          setNicknameEdit({ ...edit, failed: true });
                        }
                      }}
                      onKeyDown={e => {
                        if (e.key === 'Escape') closeNicknameEdit();
                      }}
                    >
                      <TextField
                        label={t('versions.nickname')}
                        aria-describedby="version-nickname-hint"
                        value={edit.draft}
                        onChange={e => setNicknameEdit({ ...edit, draft: e.target.value, failed: false })}
                        autoFocus
                      />
                      <p id="version-nickname-hint" className="text-sm text-ink-muted">{t('versions.nicknameHint')}</p>
                      {edit.failed && <p role="alert" className="text-sm text-danger">{t('versions.nicknameSaveFailed')}</p>}
                      <div className="flex justify-end gap-2">
                        <Button type="button" size="sm" variant="secondary" onClick={closeNicknameEdit}>
                          {t('common.cancel')}
                        </Button>
                        <Button type="submit" size="sm">{t('versions.saveNickname')}</Button>
                      </div>
                    </form>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </TabsContent>
    </Tabs>
  );
}
