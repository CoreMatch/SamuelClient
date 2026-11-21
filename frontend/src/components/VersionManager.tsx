import { useState, useEffect, useCallback } from 'react';
import { Trash2 } from 'lucide-react';
import { Asset } from '@ha/ui';
import { GetInstalledVersions } from '../../wailsjs/go/main/App';
import { installedFromDisk, versionKey, type DiskVersion, type InstalledVersion } from '../types';
import InstallWizard from './InstallWizard';
import { EmptyState, ErrorState, LoadingSkeleton } from './states';
import { t } from '../i18n';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs';
import { Badge } from '../ui/badge';
import { IconButton } from '../ui/button';

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
            {installedVersions.map((version, index) => (
              <div
                key={versionKey(version)}
                role="button"
                tabIndex={0}
                onClick={() => onSelectVersion(version)}
                onKeyDown={e => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    onSelectVersion(version);
                  }
                }}
                className={`flex w-full cursor-pointer items-center justify-between gap-3 px-5 py-5 text-left transition-colors duration-150 hover:bg-white/4 ${
                  index > 0 ? 'border-t border-edge' : ''
                }`}
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2.5">
                    <span className="text-base font-medium text-ink">{version.id}</span>
                    <Asset id="icon-check" size={20} className="text-ok" />
                  </div>
                  {version.gameDir && <p className="mt-1 break-all text-sm text-ink-muted">{version.gameDir}</p>}
                  <div className="mt-1.5 flex items-center gap-2">
                    <Badge tone={getVersionTypeTone(version.type)}>{version.type}</Badge>
                    {version.loaderType && (
                      <Badge>{`${version.loaderType} ${version.loaderVersion || ''}`}</Badge>
                    )}
                  </div>
                </div>
                {!diskIds.has(version.id) && (
                  <IconButton
                    label={t('versions.deleteLabel', { version: version.id })}
                    tone="danger"
                    onClick={e => {
                      e.stopPropagation();
                      handleDelete(version.id);
                    }}
                  >
                    <Trash2 className="size-5" />
                  </IconButton>
                )}
              </div>
            ))}
          </div>
        )}
      </TabsContent>
    </Tabs>
  );
}
