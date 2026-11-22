import { useEffect, useState } from 'react';
import { GetModrinthTargets, GetModrinthVersions, InstallModrinthMod, PreviewModrinthInstall, SearchModrinth } from '../../wailsjs/go/main/App';
import { GetCurseForgeTargets, GetCurseForgeVersions, InstallCurseForgeMod, PreviewCurseForgeInstall, SearchCurseForge } from '../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Callout } from '../ui/callout';
import { TextField } from '../ui/field';
import { Select } from '../ui/select';

interface Target {
  profile_id: string;
  game_version: string;
  loader: string;
}

interface Project {
  project_id: string;
  title: string;
  author: string;
  description: string;
}

interface Version {
  id: string;
  version_number: string;
  version_type: string;
  downloadable?: boolean;
}

interface Result {
  hits: Project[];
  limit: number;
  total_hits: number;
}

interface Plan {
  fingerprint: string;
  files: { name: string; version: string; channel: string; size: number }[];
}

function data<T>(response: { success: boolean; data: unknown; error?: string }): T {
  if (!response.success) throw new Error(response.error || t('onlineMods.failed'));
  return response.data as T;
}

function message(err: unknown) {
  return err instanceof Error ? err.message : t('onlineMods.failed');
}

export default function ModrinthBrowser({ gameDir, busy, setBusy, onInstalled, provider = 'modrinth' }: {
  gameDir: string;
  busy: boolean;
  setBusy: (value: boolean) => void;
  onInstalled: () => Promise<void>;
  provider?: 'modrinth' | 'curseforge';
}) {
  const [targets, setTargets] = useState<Target[] | null>(null);
  const [profilesFailed, setProfilesFailed] = useState(false);
  const [profile, setProfile] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState<{ query: string; offset: number; result: Result } | null>(null);
  const [project, setProject] = useState<Project | null>(null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [version, setVersion] = useState('');
  const [plan, setPlan] = useState<Plan | null>(null);
  const [working, setWorking] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState('');
  const locked = busy || working;
  const curseForge = provider === 'curseforge';
  const platform = curseForge ? 'CurseForge' : 'Modrinth';

  useEffect(() => {
    let active = true;
    const getTargets = curseForge ? GetCurseForgeTargets : GetModrinthTargets;
    void getTargets(gameDir).then(response => {
      if (!active) return;
      const list = data<Target[]>(response);
      setTargets(list);
      setProfile(list[0]?.profile_id ?? '');
    }).catch(err => {
      if (active) {
        setProfilesFailed(true);
        setError(message(err));
      }
    });
    return () => { active = false; };
  }, [gameDir, curseForge]);

  function clearProject() {
    setProject(null);
    setVersions([]);
    setVersion('');
    setPlan(null);
  }

  async function run(action: () => Promise<void>) {
    setWorking(true);
    setDone(false);
    setError('');
    try {
      await action();
    } catch (err) {
      setError(message(err));
    } finally {
      setWorking(false);
    }
  }

  function search(term: string, offset: number) {
    clearProject();
    if (offset === 0) setPage(null);
    void run(async () => {
      const searchProjects = curseForge ? SearchCurseForge : SearchModrinth;
      const result = data<Result>(await searchProjects(gameDir, profile, term, offset));
      setPage({ query: term, offset, result });
    });
  }

  function openProject(item: Project) {
    clearProject();
    void run(async () => {
      const getVersions = curseForge ? GetCurseForgeVersions : GetModrinthVersions;
      const available = data<Version[]>(await getVersions(gameDir, profile, item.project_id));
      setProject(item);
      setVersions(available);
      setVersion(available.find(item => item.downloadable !== false)?.id ?? '');
    });
  }

  function preview() {
    if (!project || !version) return;
    setPlan(null);
    void run(async () => {
      const previewInstall = curseForge ? PreviewCurseForgeInstall : PreviewModrinthInstall;
      setPlan(data<Plan>(await previewInstall(gameDir, profile, project.project_id, version)));
    });
  }

  function install() {
    if (!project || !version || !plan) return;
    setInstalling(true);
    setBusy(true);
    void run(async () => {
      try {
        const installMod = curseForge ? InstallCurseForgeMod : InstallModrinthMod;
        data(await installMod(gameDir, profile, project.project_id, version, plan.fingerprint));
        setPlan(null);
        setDone(true);
        await onInstalled();
      } finally {
        setBusy(false);
        setInstalling(false);
      }
    });
  }

  return (
    <section aria-label={platform} className="flex flex-col gap-4 rounded-md border border-edge p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="font-semibold text-ink">{t('onlineMods.platformTitle', { platform })}</h3>
        {curseForge && (
          <Button type="button" size="sm" variant="secondary" onClick={() => BrowserOpenURL('https://www.curseforge.com/minecraft/search?class=mc-mods')}>
            {t('onlineMods.openCurseForge')}
          </Button>
        )}
      </div>
      {error && <Callout tone="danger" onDismiss={() => setError('')}>{error}</Callout>}
      {done && <p role="status" className="text-sm text-ok">{t('onlineMods.installed', { platform })}</p>}
      {targets === null && !profilesFailed && <p role="status">{t('onlineMods.loadingProfiles')}</p>}
      {targets?.length === 0 && <p className="text-sm text-ink-muted">{t('onlineMods.noProfiles')}</p>}
      {targets && targets.length > 0 && (
        <>
          <p className="text-sm text-ink-muted">{t('onlineMods.shared')}</p>
          <Select
            ariaLabel={t('onlineMods.profile')}
            value={profile}
            options={targets.map(item => ({
              value: item.profile_id,
              label: `${item.game_version} / ${item.loader === 'fabric' ? 'Fabric' : 'Forge'} (${item.profile_id})`,
            }))}
            disabled={locked}
            onValueChange={value => {
              setProfile(value);
              setPage(null);
              setError('');
              setDone(false);
              clearProject();
            }}
          />
          <form className="flex flex-wrap items-end gap-3" onSubmit={event => {
            event.preventDefault();
            if (query.trim() && !locked) search(query.trim(), 0);
          }}>
            <TextField className="min-w-36 flex-1" label={t('onlineMods.query')} value={query} maxLength={80}
              disabled={locked} onChange={event => setQuery(event.target.value)} />
            <Button type="submit" disabled={locked || !query.trim()}>{t('onlineMods.search')}</Button>
          </form>
          {working && <p role="status" className="text-sm text-ink-muted">
            {t(installing ? 'onlineMods.installing' : 'onlineMods.loading', { platform })}
          </p>}
          {page?.result.hits.length === 0 && <p>{t('onlineMods.noResults', { platform })}</p>}
          {page && page.result.hits.length > 0 && (
            <>
              <ul className="grid max-h-72 gap-2 overflow-auto">
                {page.result.hits.map(item => (
                  <li key={item.project_id} className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-raised p-3">
                    <div className="min-w-0 flex-1">
                      <p className="break-words font-semibold text-ink">{item.title}</p>
                      <p className="break-words text-sm text-ink-muted">{item.author} · {item.description}</p>
                    </div>
                    <Button type="button" variant="secondary" disabled={locked} onClick={() => openProject(item)}
                      aria-label={t('onlineMods.versionsFor', { name: item.title })}>{t('onlineMods.versions')}</Button>
                  </li>
                ))}
              </ul>
              <div className="flex gap-3">
                <Button type="button" variant="secondary" disabled={locked || page.offset === 0}
                  onClick={() => search(page.query, Math.max(0, page.offset - page.result.limit))}>{t('onlineMods.previous')}</Button>
                <Button type="button" variant="secondary"
                  disabled={locked || page.offset + page.result.limit >= Math.min(page.result.total_hits, 10000)}
                  onClick={() => search(page.query, page.offset + page.result.limit)}>{t('onlineMods.next')}</Button>
              </div>
            </>
          )}
          {project && (
            <div className="flex flex-col gap-3 border-t border-edge pt-4">
              <h4 className="font-semibold text-ink">{project.title}</h4>
              {versions.length === 0 ? <p>{t('onlineMods.noVersions')}</p> : (
                <>
                  <Select
                    ariaLabel={t('onlineMods.version')}
                    value={version}
                    options={versions.map(item => ({
                      value: item.id,
                      label: `${item.version_number} (${item.version_type})${item.downloadable === false ? ` - ${t('onlineMods.websiteOnly')}` : ''}`,
                      disabled: item.downloadable === false,
                    }))}
                    disabled={locked}
                    onValueChange={value => { setVersion(value); setPlan(null); setDone(false); }}
                  />
                  {curseForge && versions.some(item => item.downloadable === false) && (
                    <p className="text-sm text-ink-muted">{t('onlineMods.curseForgeManual')}</p>
                  )}
                  <Button type="button" variant="secondary" className="self-start" disabled={locked || !version}
                    onClick={preview}>{t('onlineMods.review')}</Button>
                </>
              )}
              {plan && (
                <div className="flex flex-col gap-3 rounded-md bg-raised p-4">
                  <p className="font-semibold text-ink">{t('onlineMods.confirmTitle', { platform })}</p>
                  <ul className="flex flex-col gap-1 text-sm text-ink-muted">
                    {plan.files.map((file, index) => (
                      <li key={file.name} className="break-all">
                        {t(index === 0 ? 'onlineMods.mainFile' : 'onlineMods.dependency')}: {file.name}
                        {' '}({file.version}, {file.channel}, {Math.max(1, Math.round(file.size / 1024))} KiB)
                      </li>
                    ))}
                  </ul>
                  <div className="flex flex-wrap gap-3">
                    <Button type="button" loading={installing} disabled={locked} onClick={install}>{t('onlineMods.confirm')}</Button>
                    <Button type="button" variant="secondary" disabled={locked}
                      onClick={() => setPlan(null)}>{t('common.cancel')}</Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </>
      )}
    </section>
  );
}
