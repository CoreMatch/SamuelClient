import { useState, useEffect, useRef } from 'react';
import { Asset, patternStyle } from '@ha/ui';
import type { StoredAccount } from '../accountStorage';
import type { InstalledVersion } from '../types';
import { t } from '../i18n';
import GetStartedChecklist from './GetStartedChecklist';
import { Button } from '../ui/button';
import { Select } from '../ui/select';
import { Field, NumberField, TextField } from '../ui/field';
import { Progress, Slider } from '../ui/progress';
import { Callout } from '../ui/callout';
import { Badge } from '../ui/badge';
import { useInstallProgress, installStepKey, stageIcon } from '../useInstallProgress';
import {
  GetJavaInstallations,
  GetInstalledVersions,
  GetFabricVersions,
  GetForgeVersions,
  InstallFabricLoader,
  InstallForgeLoader,
  LaunchGame,
  CancelInstall,
} from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

interface ModLoader {
  name: string;
  version: string;
}

// One loader install attempt; kept around after a cancel so the retry button
// can re-run the exact same install.
interface LoaderInstallTarget {
  loaderName: string;
  mcVersionId: string;
  loaderVersion: string;
  isFabric: boolean;
}

// Shapes of the backend payloads consumed below (see bindings.go).
interface BackendInstalledVersion {
  id: string;
  type: string;
  has_json: boolean;
  has_jar: boolean;
}

interface FabricLoaderEntry {
  loader: { version: string };
}

interface ForgeVersionEntry {
  version: string;
}

interface JavaInstallation {
  id: number;
  path: string;
  friendly_name: string;
  version: string;
  is_default: boolean;
}

// LiveLogLine is one line of game output streamed over the "launcher:log"
// Wails event (ADR-0004).
interface LiveLogLine {
  source: string;
  line: string;
  ts: string;
}

interface HomeViewProps {
  selectedVersion: InstalledVersion | null;
  onVersionSelect: (version: InstalledVersion | null) => void;
  currentAccount: StoredAccount | null;
  onOpenSettings: () => void;
}

const STORAGE_KEY = 'launcher_settings';

interface LauncherSettings {
  maxMemory: number;
  minMemory: number;
  javaArgs: string;
  gameWidth: number;
  gameHeight: number;
  selectedLoader: string;
  loaderVersion: string;
}

const DEFAULT_SETTINGS: LauncherSettings = {
  maxMemory: 4096,
  minMemory: 1024,
  javaArgs: '',
  gameWidth: 854,
  gameHeight: 480,
  selectedLoader: 'none',
  loaderVersion: '',
};

export default function HomeView({
  selectedVersion,
  onVersionSelect,
  currentAccount,
  onOpenSettings,
}: HomeViewProps) {
  const [versions, setVersions] = useState<InstalledVersion[]>([]);
  const [currentVersion, setCurrentVersion] = useState<InstalledVersion | null>(selectedVersion);
  const [loaders, setLoaders] = useState<ModLoader[]>([]);
  const [settings, setSettings] = useState<LauncherSettings>(DEFAULT_SETTINGS);
  const [launching, setLaunching] = useState(false);
  const [error, setError] = useState('');
  const [liveLogs, setLiveLogs] = useState<LiveLogLine[]>([]);
  const [showLogs, setShowLogs] = useState(false);
  const [javaInstallations, setJavaInstallations] = useState<JavaInstallation[]>([]);
  const [selectedJava, setSelectedJava] = useState<JavaInstallation | null>(null);
  const [installingLoader, setInstallingLoader] = useState<string | null>(null);
  // installingLoaderId is the backend version id of the in-flight loader
  // install (same scheme as InstallWizard); it targets CancelInstall and
  // picks the matching download:progress entry. loaderCancelled keeps the
  // cancelled target so the retry button can re-run the same install.
  const [installingLoaderId, setInstallingLoaderId] = useState('');
  const [loaderCancelled, setLoaderCancelled] = useState<LoaderInstallTarget | null>(null);
  const cancelRequestedRef = useRef(false);
  // download:progress subscription shared with the InstallWizard.
  const { progress } = useInstallProgress();
  // First-run checklist state: the game has launched at least once.
  const [hasLaunched, setHasLaunched] = useState(() => localStorage.getItem('has_launched_game') === '1');

  useEffect(() => {
    loadVersions();
    loadSettings();
    loadJavaInstallations();
  }, []);

  // ADR-0004: live launch logs are pushed by the backend as "launcher:log"
  // events ({source: "stdout"|"stderr", line: string}); subscribe once and
  // unsubscribe on unmount. No HTTP polling involved.
  useEffect(() => {
    EventsOn('launcher:log', (payload: { source?: string; line?: string }) => {
      if (!payload) return;
      setLiveLogs(prev => [
        ...prev,
        {
          source: payload.source || 'stdout',
          line: payload.line || '',
          ts: new Date().toLocaleTimeString(),
        },
      ]);
    });
    return () => {
      EventsOff('launcher:log');
    };
  }, []);

  const loadJavaInstallations = async () => {
    try {
      const result = await GetJavaInstallations();
      if (result.success && Array.isArray(result.data)) {
        const data = result.data as JavaInstallation[];
        setJavaInstallations(data);
        const defaultJava = data.find((j: JavaInstallation) => j.is_default);
        if (defaultJava) {
          setSelectedJava(defaultJava);
        } else if (data.length > 0) {
          setSelectedJava(data[0]);
        }
      }
    } catch (err) {
      console.error('Failed to load Java installations:', err);
    }
  };

  useEffect(() => {
    if (currentVersion) {
      onVersionSelect(currentVersion);
    }
  }, [currentVersion, onVersionSelect]);

  const loadVersions = async () => {
    try {
      // Prefer localStorage (lets the user mark versions as installed for modded setups),
      // then fall back to a disk scan from the backend for any version that has a JSON+jar.
      const stored = localStorage.getItem('installed_versions');
      const local: InstalledVersion[] = stored ? JSON.parse(stored) : [];

      try {
        const result = await GetInstalledVersions();
        if (result.success && Array.isArray(result.data?.versions)) {
          const backendVersions = result.data.versions as BackendInstalledVersion[];
          const fromDisk: InstalledVersion[] = backendVersions
            .filter(v => v.has_json && v.has_jar)
            .map(v => ({
              id: v.id,
              type: v.type || 'release',
              installed: true,
            }));
          // merge, keep localStorage order, dedupe by id
          const seen = new Set(local.map(v => v.id));
          const merged = [...local, ...fromDisk.filter(v => !seen.has(v.id))];
          setVersions(merged);
          return;
        }
      } catch (e) {
        console.error('Failed to query installed versions from backend:', e);
      }
      setVersions(local);
    } catch (err) {
      console.error('Failed to load versions:', err);
    }
  };

  const loadSettings = () => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        setSettings({ ...DEFAULT_SETTINGS, ...JSON.parse(stored) });
      }
    } catch (err) {
      console.error('Failed to load settings:', err);
    }
  };

  const saveSettings = (newSettings: LauncherSettings) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(newSettings));
      setSettings(newSettings);
    } catch (err) {
      console.error('Failed to save settings:', err);
    }
  };

  const fetchLoaders = async (mcVersion: string) => {
    try {
      const [fabricResult, forgeResult] = await Promise.all([
        GetFabricVersions(mcVersion),
        GetForgeVersions(mcVersion),
      ]);

      const loaderList: ModLoader[] = [{ name: 'none', version: '' }];

      if (fabricResult.success && Array.isArray(fabricResult.data)) {
        (fabricResult.data as FabricLoaderEntry[]).slice(0, 5).forEach(f => {
          loaderList.push({
            name: `fabric-${f.loader.version}`,
            version: f.loader.version,
          });
        });
      }

      if (forgeResult.success && Array.isArray(forgeResult.data)) {
        (forgeResult.data as ForgeVersionEntry[]).slice(0, 5).forEach(f => {
          loaderList.push({
            name: `forge-${f.version}`,
            version: f.version,
          });
        });
      }

      setLoaders(loaderList);
    } catch (err) {
      console.error('Failed to fetch loaders:', err);
    }
  };

  const handleVersionChange = (versionId: string) => {
    const version = versions.find(v => v.id === versionId);
    setCurrentVersion(version || null);
    if (version) {
      fetchLoaders(version.id);
    }
  };

  const handleLoaderChange = async (loaderName: string) => {
    const loader = loaders.find(l => l.name === loaderName);
    saveSettings({
      ...settings,
      selectedLoader: loaderName,
      loaderVersion: loader?.version || '',
    });

    if (!loader || loader.name === 'none' || !loader.version) {
      return;
    }
    if (!currentVersion) {
      setError(t('play.selectVersionFirst'));
      return;
    }

    // Loader versions install through the dedicated bindings: Fabric writes a
    // profile JSON inheriting the vanilla version, Forge runs its installer.
    // The resulting version shows up in the version list afterwards.
    await startLoaderInstall({
      loaderName: loader.name,
      mcVersionId: currentVersion.id,
      loaderVersion: loader.version,
      isFabric: loader.name.startsWith('fabric-'),
    });
  };

  // loaderVersionId mirrors the backend's version id scheme for loader
  // installs (bindings.go), so it targets CancelInstall and matches the
  // download:progress entries.
  const loaderVersionId = (target: LoaderInstallTarget) =>
    target.isFabric
      ? `fabric-loader-${target.loaderVersion}-${target.mcVersionId}`
      : `${target.mcVersionId}-forge-${target.loaderVersion}`;

  // startLoaderInstall runs one loader install attempt with the same
  // semantics as the InstallWizard: progress comes from the shared
  // useInstallProgress subscription, a user cancel finalizes to the
  // retryable cancelled state instead of an error, and the backend's
  // failure cleanup keeps retries safe.
  const startLoaderInstall = async (target: LoaderInstallTarget) => {
    setError('');
    setLoaderCancelled(null);
    cancelRequestedRef.current = false;
    setInstallingLoader(target.loaderName);
    setInstallingLoaderId(loaderVersionId(target));
    try {
      const result = target.isFabric
        ? await InstallFabricLoader(target.mcVersionId, target.loaderVersion, '')
        : await InstallForgeLoader(target.mcVersionId, target.loaderVersion, '');
      if (result.success) {
        await loadVersions();
      } else if (cancelRequestedRef.current) {
        // A user-initiated cancel finalizes cleanly rather than surfacing a
        // failure; the backend already ran its failure cleanup so a retry
        // is safe.
        setLoaderCancelled(target);
      } else {
        setError(result.error || t('play.loaderInstallFailed'));
      }
    } catch {
      if (cancelRequestedRef.current) {
        setLoaderCancelled(target);
      } else {
        setError(t('play.loaderInstallFailed'));
      }
    } finally {
      setInstallingLoader(null);
      setInstallingLoaderId('');
    }
  };

  const cancelLoaderInstall = () => {
    if (!installingLoaderId) return;
    cancelRequestedRef.current = true;
    void CancelInstall(installingLoaderId);
  };

  const handleSettingChange = (key: keyof LauncherSettings, value: number | string) => {
    saveSettings({
      ...settings,
      [key]: value,
    });
  };

  const handleLaunch = async () => {
    if (!currentVersion || !currentAccount) {
      setError(t('play.selectVersionAccount'));
      return;
    }
    // Tokens are hydrated from the OS keychain (ADR-0008); a session without
    // them must re-authenticate instead of launching.
    if (!currentAccount.accessToken || !currentAccount.clientToken) {
      setError(t('accounts.reLoginNeeded'));
      return;
    }

    setLaunching(true);
    setError('');
    setShowLogs(true);
    setLiveLogs([]);

    try {
      // ADR-0004: the launch request (including access/client tokens) goes
      // through the in-process Wails binding, not a local HTTP socket. Live
      // game output arrives separately over "launcher:log" events.
      const result = await LaunchGame({
        version_id: currentVersion.id,
        version_type: currentVersion.type,
        username: currentAccount.username,
        uuid: currentAccount.uuid,
        access_token: currentAccount.accessToken,
        client_token: currentAccount.clientToken,
        auth_type: currentAccount.authType,
        authlib_jar: localStorage.getItem('authlib_jar_path') || '',
        authlib_meta: '',
        game_dir: '',
        java_path: '',
        loader_type: settings.selectedLoader === 'none' ? '' : settings.selectedLoader.split('-')[0],
        loader_version: settings.loaderVersion,
        join_server: '',
        join_port: 0,
        max_memory: settings.maxMemory,
        min_memory: settings.minMemory,
        java_args: settings.javaArgs,
        game_width: settings.gameWidth,
        game_height: settings.gameHeight,
      });

      if (result.success) {
        console.log('Game launched with PID:', result.data?.pid);
        localStorage.setItem('has_launched_game', '1');
        setHasLaunched(true);
      } else {
        setError(result.error || t('play.launchFailed'));
      }
    } catch {
      setError(t('play.launchFailed'));
    } finally {
      setLaunching(false);
    }
  };

  // Progress entry for the in-flight loader install (shared subscription);
  // download stages carry a real byte percentage, the loader run itself is
  // indeterminate until its files report totals.
  const loaderProgress = installingLoaderId ? progress[installingLoaderId] : undefined;

  return (
    <div className="flex flex-col gap-8">
      <GetStartedChecklist
        hasAccount={!!currentAccount}
        installedCount={versions.length}
        hasLaunched={hasLaunched}
      />

      {error && (
        <Callout tone="danger" onDismiss={() => setError('')}>
          {error}
        </Callout>
      )}

      <section className="rounded-lg bg-panel p-8" style={patternStyle('pattern-grid-fine')}>
        <div className="flex flex-col gap-6">
          <Field label={t('play.version')} htmlFor="play-version">
            <Select
              ariaLabel={t('play.version')}
              value={currentVersion?.id || ''}
              onValueChange={handleVersionChange}
              placeholder={t('play.noVersions')}
              options={versions.map(v => ({ value: v.id, label: v.id }))}
            />
          </Field>

          <Field label={t('play.loader')} htmlFor="play-loader">
            <Select
              ariaLabel={t('play.loader')}
              value={settings.selectedLoader}
              onValueChange={handleLoaderChange}
              disabled={installingLoader !== null}
              options={loaders.map(l => ({
                value: l.name,
                label: l.name === 'none' ? t('play.none') : l.name.replace('-', ' '),
              }))}
            />
          </Field>

          {installingLoader && (
            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="inline-flex items-center gap-2 text-sm text-ink-muted">
                  {stageIcon(loaderProgress?.stage || 'loader')}
                  {t(installStepKey(loaderProgress?.stage || 'loader'))}
                </span>
                {loaderProgress?.pct != null && (
                  <span className="font-mono text-sm text-ink-muted">{loaderProgress.pct}%</span>
                )}
              </div>
              <Progress value={loaderProgress?.pct ?? null} />
              <div className="mt-3 flex justify-end">
                <Button variant="secondary" onClick={cancelLoaderInstall}>
                  {t('common.cancel')}
                </Button>
              </div>
            </div>
          )}

          {loaderCancelled && (
            <div className="flex items-center gap-4">
              <div className="flex-1">
                <Callout tone="warning">{t('wizard.installCancelled')}</Callout>
              </div>
              <Button variant="secondary" onClick={() => startLoaderInstall(loaderCancelled)}>
                {t('common.retry')}
              </Button>
            </div>
          )}

          <Field label={t('play.java')} htmlFor="play-java">
            <Select
              ariaLabel={t('play.java')}
              value={selectedJava ? String(selectedJava.id) : ''}
              onValueChange={value => {
                const java = javaInstallations.find(j => String(j.id) === value);
                setSelectedJava(java || null);
              }}
              placeholder={t('play.noJava')}
              options={javaInstallations.map(j => ({
                value: String(j.id),
                label: `${j.friendly_name || `Java ${j.version}`} (${j.version})`,
              }))}
            />
          </Field>
        </div>

        <div className="mt-8 flex items-center justify-between border-t border-edge pt-8">
          <span className="text-sm font-medium text-ink-muted">{t('play.account')}</span>
          {currentAccount ? (
            <span className="flex items-center gap-3">
              <span className="text-base font-medium text-ink">{currentAccount.username}</span>
              <Badge tone="success">{t('play.hrpauth')}</Badge>
            </span>
          ) : (
            <span className="text-sm text-danger">{t('play.noAccount')}</span>
          )}
        </div>

        <div className="mt-8 border-t border-edge pt-8">
          <h3 className="text-sm font-medium text-ink-muted">{t('play.launchSettings')}</h3>

          <div className="mt-5 flex flex-col gap-6">
            <div>
              <div className="mb-3 flex items-baseline justify-between">
                <span className="text-sm text-ink-muted">{t('play.maxMemory')}</span>
                <span className="font-mono text-base text-ink">{settings.maxMemory} MB</span>
              </div>
              <Slider
                ariaLabel={t('play.maxMemory')}
                value={settings.maxMemory}
                onValueChange={value => handleSettingChange('maxMemory', value)}
                min={512}
                max={16384}
                step={256}
              />
            </div>

            <div className="grid grid-cols-2 gap-6">
              <NumberField
                label={t('play.width')}
                value={settings.gameWidth}
                min={1}
                onChange={e => handleSettingChange('gameWidth', parseInt(e.target.value) || 854)}
              />
              <NumberField
                label={t('play.height')}
                value={settings.gameHeight}
                min={1}
                onChange={e => handleSettingChange('gameHeight', parseInt(e.target.value) || 480)}
              />
            </div>

            <TextField
              label={t('play.jvmArgs')}
              value={settings.javaArgs}
              onChange={e => handleSettingChange('javaArgs', e.target.value)}
              placeholder="-XX:+UseG1GC"
              className="font-mono"
            />
          </div>
        </div>

        <div className="mt-8 flex items-center gap-4">
          <Button variant="secondary" size="lg" onClick={onOpenSettings} className="h-14">
            {t('nav.settings')}
          </Button>
          <Button
            size="lg"
            className="h-14 flex-1 text-lg"
            onClick={handleLaunch}
            disabled={launching || !currentVersion || !currentAccount}
            loading={launching}
          >
            {!launching && <Asset id="icon-launch" size={20} />}
            {launching ? t('play.launching') : t('play.play')}
          </Button>
        </div>
      </section>

      {showLogs && (
        <section className="rounded-lg bg-panel p-8">
          <h3 className="text-xl font-semibold text-ink">{t('play.logs')}</h3>
          <div className="mt-4 h-64 overflow-y-auto rounded-md bg-base p-5 font-mono text-sm">
            {liveLogs.length > 0 ? (
              // ADR-0004: live game output streamed over "launcher:log".
              <div className="flex flex-col gap-1.5">
                {liveLogs.map((l, i) => (
                  <div key={i} className="flex gap-3">
                    <span className="shrink-0 text-ink-faint">{l.ts}</span>
                    <span
                      className={`shrink-0 font-semibold ${l.source === 'stderr' ? 'text-warn' : 'text-info'}`}
                    >
                      [{l.source.toUpperCase()}]
                    </span>
                    <span className="break-all whitespace-pre-wrap text-ink-muted">{l.line}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="py-10 text-center text-ink-faint">{t('play.noLogs')}</p>
            )}
          </div>
        </section>
      )}
    </div>
  );
}
