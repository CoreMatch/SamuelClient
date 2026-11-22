import { useEffect, useState } from 'react';
import {
  GetFabricVersions,
  GetForgeVersions,
  InstallFabricLoader,
  InstallForgeLoader,
  RemoveLoaderProfile,
} from '../../wailsjs/go/main/App';
import type { InstalledVersion } from '../types';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Callout } from '../ui/callout';
import { Field } from '../ui/field';
import { Select } from '../ui/select';

interface VersionLoaderManagerProps {
  version: InstalledVersion;
  onChanged: () => Promise<void>;
}

interface InstalledLoader {
  type: 'fabric' | 'forge';
  version: string;
}

function detectLoader(versionID: string): InstalledLoader | null {
  const fabric = versionID.match(/^fabric-loader-(.+)-([^-]+)$/);
  if (fabric) return { type: 'fabric', version: fabric[1] };
  const forge = versionID.match(/^(.+)-forge-?(.+)$/);
  if (forge) return { type: 'forge', version: forge[2] };
  return null;
}

function getMinecraftVersion(version: InstalledVersion): string {
  if (version.inheritsFrom) return version.inheritsFrom;
  const fabric = version.id.match(/^fabric-loader-(.+)-(.+)$/);
  if (fabric) return fabric[2];
  const forge = version.id.match(/^(.+)-forge-?(.+)$/);
  return forge ? forge[1] : version.id;
}

export default function VersionLoaderManager({ version, onChanged }: VersionLoaderManagerProps) {
  const [loaderType, setLoaderType] = useState<'fabric' | 'forge'>('fabric');
  const [fabricVersions, setFabricVersions] = useState<string[]>([]);
  const [forgeVersions, setForgeVersions] = useState<string[]>([]);
  const [fabricVersion, setFabricVersion] = useState('');
  const [forgeVersion, setForgeVersion] = useState('');
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const minecraftVersion = getMinecraftVersion(version);
  const installedLoader = detectLoader(version.id);
  const availableVersions = loaderType === 'fabric' ? fabricVersions : forgeVersions;
  const selectedVersion = loaderType === 'fabric' ? fabricVersion : forgeVersion;

  useEffect(() => {
    let cancelled = false;
    setCatalogLoading(true);
    setError('');
    Promise.all([
      GetFabricVersions(minecraftVersion),
      GetForgeVersions(minecraftVersion),
    ]).then(([fabricResult, forgeResult]) => {
      if (cancelled) return;
      const fabric = fabricResult.success && Array.isArray(fabricResult.data)
        ? (fabricResult.data as { loader: { version: string } }[]).map(item => item.loader.version)
        : [];
      const forge = forgeResult.success && Array.isArray(forgeResult.data)
        ? (forgeResult.data as { version: string }[]).map(item => item.version)
        : [];
      setFabricVersions(fabric);
      setForgeVersions(forge);
      setFabricVersion(current => fabric.includes(current) ? current : fabric[0] || '');
      setForgeVersion(current => forge.includes(current) ? current : forge[0] || '');
    }).catch(() => {
      if (!cancelled) setError(t('common.loadError', { data: t('wizard.loaderVersionsData') }));
    }).finally(() => {
      if (!cancelled) setCatalogLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [minecraftVersion]);

  const addLoader = async () => {
    setBusy(true);
    setError('');
    try {
      const result = loaderType === 'fabric'
        ? await InstallFabricLoader(minecraftVersion, fabricVersion, version.gameDir ?? '')
        : await InstallForgeLoader(minecraftVersion, forgeVersion, version.gameDir ?? '');
      if (!result.success) throw new Error(result.error || 'Loader installation failed');
      await onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Loader installation failed');
    } finally {
      setBusy(false);
    }
  };

  const removeLoader = async () => {
    if (!installedLoader) return;
    const label = `${installedLoader.type === 'fabric' ? 'Fabric' : 'Forge'} ${installedLoader.version}`;
    if (!window.confirm(t('versions.removeLoaderConfirm', { loader: label }))) return;
    setBusy(true);
    setError('');
    try {
      const result = await RemoveLoaderProfile(version.id, version.gameDir ?? '');
      if (!result.success) throw new Error(result.error || 'Loader removal failed');
      await onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Loader removal failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4 border-t border-edge px-5 py-5">
      {error && <Callout tone="danger">{error}</Callout>}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('versions.loaderToAdd')} htmlFor="installed-loader-type">
          <Select
            ariaLabel={t('versions.loaderToAdd')}
            value={loaderType}
            onValueChange={value => setLoaderType(value as 'fabric' | 'forge')}
            options={[
              { value: 'fabric', label: 'Fabric' },
              { value: 'forge', label: 'Forge' },
            ]}
            disabled={busy}
          />
        </Field>
        <Field label={t('versions.loaderVersion')} htmlFor="installed-loader-version">
          <Select
            ariaLabel={t('versions.loaderVersion')}
            value={selectedVersion}
            onValueChange={loaderType === 'fabric' ? setFabricVersion : setForgeVersion}
            placeholder={catalogLoading ? t('versions.loaderCatalogLoading') : t('versions.loaderCatalogEmpty')}
            options={availableVersions.map(loaderVersion => ({
              value: loaderVersion,
              label: loaderVersion,
            }))}
            disabled={catalogLoading || busy || availableVersions.length === 0}
          />
        </Field>
      </div>
      {availableVersions.length === 0 && !catalogLoading && (
        <p className="text-sm text-ink-muted">{t('versions.loaderCatalogEmpty')}</p>
      )}
      <div className="flex flex-wrap justify-end gap-3">
        {installedLoader && (
          <Button type="button" variant="danger" onClick={removeLoader} disabled={busy}>
            {t('versions.removeLoader')}
          </Button>
        )}
        <Button
          type="button"
          onClick={addLoader}
          loading={busy}
          disabled={catalogLoading || !selectedVersion}
        >
          {t(busy
            ? 'versions.installingLoader'
            : installedLoader ? 'versions.addAnotherLoader' : 'versions.addLoader')}
        </Button>
      </div>
    </div>
  );
}
