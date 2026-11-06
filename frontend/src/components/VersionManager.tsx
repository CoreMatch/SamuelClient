import { useState, useEffect, useCallback } from 'react';
import { CircleCheck, Download, Trash2 } from 'lucide-react';
import type { InstalledVersion } from '../types';
import InstallWizard from './InstallWizard';
import { EmptyState } from './states';
import { t } from '../i18n';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs';
import { Badge } from '../ui/badge';
import { IconButton } from '../ui/button';

interface VersionManagerProps {
  onSelectVersion: (version: InstalledVersion) => void;
}

const STORAGE_KEY = 'installed_versions';

// VersionManager hosts the three-step install wizard (research 03 §8.3)
// and the installed-version list. Installations run through the wizard;
// this component owns the localStorage working copy of installed versions.
export default function VersionManager({ onSelectVersion }: VersionManagerProps) {
  const [installedVersions, setInstalledVersions] = useState<InstalledVersion[]>([]);
  const [tabValue, setTabValue] = useState('install');

  const loadInstalledVersions = useCallback(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        setInstalledVersions(JSON.parse(stored));
      }
    } catch (err) {
      console.error('Failed to load installed versions:', err);
    }
  }, []);

  useEffect(() => {
    loadInstalledVersions();
  }, [loadInstalledVersions]);

  const handleDelete = (versionId: string) => {
    const updated = installedVersions.filter(v => v.id !== versionId);
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
    } catch (err) {
      console.error('Failed to save installed versions:', err);
    }
    setInstalledVersions(updated);
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
    <Tabs value={tabValue} onValueChange={setTabValue}>
      <TabsList>
        <TabsTrigger value="install">{t('versions.tab.install')}</TabsTrigger>
        <TabsTrigger value="installed">{t('versions.tab.installed')}</TabsTrigger>
      </TabsList>

      <TabsContent value="install">
        <InstallWizard onInstalled={loadInstalledVersions} />
      </TabsContent>

      <TabsContent value="installed">
        {installedVersions.length === 0 ? (
          <EmptyState
            icon={<Download className="size-12 opacity-60" aria-hidden />}
            title={t('versions.emptyTitle')}
            hint={t('versions.emptyHint')}
            actionLabel={t('versions.browse')}
            onAction={() => setTabValue('install')}
          />
        ) : (
          <div className="rounded-lg border border-edge bg-panel">
            {installedVersions.map((version, index) => (
              <div
                key={version.id}
                role="button"
                tabIndex={0}
                onClick={() => onSelectVersion(version)}
                onKeyDown={e => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    onSelectVersion(version);
                  }
                }}
                className={`flex w-full cursor-pointer items-center justify-between gap-3 px-4 py-3 text-left transition-colors duration-150 hover:bg-white/4 ${
                  index > 0 ? 'border-t border-edge' : ''
                }`}
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-ink">{version.id}</span>
                    <CircleCheck className="size-4 text-ok" aria-hidden />
                  </div>
                  <div className="mt-1 flex items-center gap-1.5">
                    <Badge tone={getVersionTypeTone(version.type)}>{version.type}</Badge>
                    {version.loaderType && (
                      <Badge>{`${version.loaderType} ${version.loaderVersion || ''}`}</Badge>
                    )}
                  </div>
                </div>
                <IconButton
                  label={t('versions.deleteLabel', { version: version.id })}
                  tone="danger"
                  onClick={e => {
                    e.stopPropagation();
                    handleDelete(version.id);
                  }}
                >
                  <Trash2 className="size-4" />
                </IconButton>
              </div>
            ))}
          </div>
        )}
      </TabsContent>
    </Tabs>
  );
}
