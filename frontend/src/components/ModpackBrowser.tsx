import { useState } from 'react';
import {
  CancelInstall, GetModrinthModpackVersions, InstallModpack, PickModpack, PreviewModpack, SearchModrinthModpacks,
} from '../../wailsjs/go/main/App';
import { t } from '../i18n';
import { recommendJavaMajor } from '../javaRuntimeRecommendation';
import { type DiskVersion, type InstalledVersion, installedFromDisk } from '../types';
import { useInstallProgress } from '../useInstallProgress';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';
import { Progress } from '../ui/progress';
import { Select } from '../ui/select';

interface Project {
  project_id: string;
  title: string;
  description: string;
  author: string;
}

interface SearchPage {
  hits: Project[];
  total_hits: number;
  limit: number;
}

interface Version {
  id: string;
  version_number: string;
  version_type: string;
  game_versions: string[];
  loaders: string[];
}

interface Preview {
  name: string;
  version: string;
  game_version: string;
  loader: string;
  loader_version: string;
  required_bytes: number;
  options: { path: string; size: number }[];
  fingerprint: string;
}

type Source = [projectId: string, versionId: string, localPath: string];

function unwrap<T>(response: { success: boolean; data?: unknown; error?: string }): T {
  if (!response.success) throw new Error(response.error || t('packs.error'));
  return response.data as T;
}

function taskId() {
  return 'modpack-' + Array.from(crypto.getRandomValues(new Uint8Array(16)),
    byte => byte.toString(16).padStart(2, '0')).join('');
}

export default function ModpackBrowser({ onInstalled, onSelectVersion }: {
  onInstalled: () => void;
  onSelectVersion: (version: InstalledVersion) => void;
}) {
  const [query, setQuery] = useState('');
  const [page, setPage] = useState<{ query: string; offset: number; result: SearchPage }>();
  const [project, setProject] = useState<Project>();
  const [versions, setVersions] = useState<Version[]>([]);
  const [version, setVersion] = useState('');
  const [plan, setPlan] = useState<{ source: Source; preview: Preview }>();
  const [optional, setOptional] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [task, setTask] = useState('');
  const [error, setError] = useState('');
  const [installed, setInstalled] = useState<InstalledVersion>();
  const { progress } = useInstallProgress();

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError('');
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  function search(term: string, offset: number) {
    setProject(undefined);
    setPlan(undefined);
    setInstalled(undefined);
    void run(async () => {
      const result = unwrap<SearchPage>(await SearchModrinthModpacks(term, offset));
      setPage({ query: term, offset, result });
    });
  }

  function openProject(item: Project) {
    setProject(item);
    setVersions([]);
    setVersion('');
    setPlan(undefined);
    setInstalled(undefined);
    void run(async () => {
      const rows = unwrap<Version[]>(await GetModrinthModpackVersions(item.project_id));
      setVersions(rows);
      setVersion(rows[0]?.id ?? '');
    });
  }

  async function preview(source: Source) {
    setPlan(undefined);
    setInstalled(undefined);
    setOptional([]);
    const details = unwrap<Preview>(await PreviewModpack(...source));
    setPlan({ source, preview: details });
  }

  function install() {
    if (!plan) return;
    const id = taskId();
    setTask(id);
    void run(async () => {
      try {
        const result = unwrap<{ name: string; version_id: string; game_dir: string }>(
          await InstallModpack(id, ...plan.source, plan.preview.fingerprint, optional),
        );
        const row: DiskVersion = {
          id: result.version_id, type: 'release', has_json: true, has_jar: true,
          game_dir: result.game_dir, modpack_name: result.name,
        };
        setInstalled(installedFromDisk([row])[0]);
        setPlan(undefined);
        onInstalled();
      } finally {
        setTask('');
      }
    });
  }

  const detail = plan?.preview;
  const java = detail ? recommendJavaMajor(detail.game_version) : null;
  const packBytes = detail ? detail.required_bytes + detail.options.reduce(
    (sum, item) => sum + (optional.includes(item.path) ? item.size : 0), 0,
  ) : 0;
  return (
    <section className="space-y-5 rounded-lg bg-panel p-5">
      <form className="flex flex-wrap items-end gap-3" onSubmit={e => { e.preventDefault(); search(query, 0); }}>
        <TextField label={t('packs.query')} className="min-w-48 flex-1" value={query} maxLength={256}
          disabled={busy} onChange={e => setQuery(e.target.value)} />
        <Button type="submit" disabled={busy}>{t('packs.search')}</Button>
        <Button type="button" variant="secondary" disabled={busy} onClick={() => void run(async () => {
          const path = unwrap<string>(await PickModpack());
          if (path) {
            setProject(undefined);
            await preview(['', '', path]);
          }
        })}>{t('packs.local')}</Button>
      </form>

      {page && (
        <div className="space-y-3">
          {!page.result.hits.length && <p className="text-ink-muted">{t('packs.noResults')}</p>}
          <ul className="max-h-60 space-y-2 overflow-y-auto">
            {page.result.hits.map(item => (
              <li key={item.project_id}>
                <button type="button" disabled={busy} onClick={() => openProject(item)}
                  aria-pressed={project?.project_id === item.project_id}
                  className="w-full rounded-md border border-edge p-3 text-left hover:bg-raised">
                  <span className="block font-medium text-ink">{item.title}</span>
                  <span className="block text-sm text-ink-muted">{item.author} · {item.description}</span>
                </button>
              </li>
            ))}
          </ul>
          <div className="flex gap-2">
            {page.offset > 0 && <Button size="sm" variant="secondary" disabled={busy}
              onClick={() => search(page.query, Math.max(0, page.offset - page.result.limit))}>{t('packs.previous')}</Button>}
            {page.offset + page.result.limit < Math.min(10000, page.result.total_hits) &&
              <Button size="sm" variant="secondary" disabled={busy}
                onClick={() => search(page.query, page.offset + page.result.limit)}>{t('packs.next')}</Button>}
          </div>
        </div>
      )}

      {project && (
        <div className="space-y-3 border-t border-edge pt-4">
          <h3 className="font-semibold text-ink">{project.title}</h3>
          {versions.length === 0 ? (!busy && <p className="text-ink-muted">{t('packs.noVersions')}</p>) : (
            <div className="flex flex-wrap gap-3">
              <Select ariaLabel={t('packs.version')} value={version} disabled={busy}
                className="max-w-full flex-1"
                options={versions.map(row => ({
                  value: row.id, label: `${row.version_number} (${row.version_type}) · ${row.loaders.join(', ')} · ${row.game_versions.join(', ')}`,
                }))}
                onValueChange={id => { setVersion(id); setPlan(undefined); }} />
              <Button variant="secondary" disabled={busy || !version}
                onClick={() => void run(() => preview([project.project_id, version, '']))}>{t('packs.preview')}</Button>
            </div>
          )}
        </div>
      )}

      {detail && (
        <div className="space-y-3 border-t border-edge pt-4">
          <h3 className="text-lg font-semibold text-ink">{detail.name} · {detail.version}</h3>
          <p className="text-ink-muted">Minecraft {detail.game_version} · {detail.loader} {detail.loader_version}</p>
          <p className="text-sm text-ink-muted">{t('packs.downloadSize', { size: (packBytes / 1048576).toFixed(1) })}</p>
          {java && <p className="text-sm text-ink-muted">{t('packs.java', { major: String(java.major) })}</p>}
          <p className="text-sm text-ink-muted">{t('packs.trust')}</p>
          {detail.options.length > 0 && (
            <fieldset disabled={busy} className="max-h-48 space-y-2 overflow-y-auto text-sm text-ink">
              <legend className="mb-2 font-medium">{t('packs.optional')}</legend>
              {detail.options.map(item => (
                <label key={item.path} className="flex items-center gap-2 break-all">
                  <input type="checkbox" checked={optional.includes(item.path)}
                    onChange={e => setOptional(current => e.target.checked ? [...current, item.path] : current.filter(p => p !== item.path))} />
                  {item.path}
                </label>
              ))}
            </fieldset>
          )}
          <Button disabled={busy} onClick={install}>{t('packs.install')}</Button>
        </div>
      )}
      {task && (
        <div className="flex items-center gap-3">
          <div className="flex-1" role="status" aria-label={t('packs.installing')}>
            <p className="mb-2 text-sm text-ink-muted">{t('packs.installing')}</p>
            <Progress value={progress[task]?.pct ?? null} />
          </div>
          <Button variant="secondary" onClick={() => void CancelInstall(task).catch(() => {})}>{t('common.cancel')}</Button>
        </div>
      )}
      {busy && !task && <p role="status" className="text-sm text-ink-muted">{t('packs.loading')}</p>}
      {error && <p role="alert" className="text-danger">{error}</p>}
      {installed && (
        <div className="flex flex-wrap items-center gap-3">
          <p role="status" className="text-ok">{t('packs.installed', { name: installed.modpackName ?? installed.id })}</p>
          <Button onClick={() => onSelectVersion(installed)}>{t('packs.select')}</Button>
        </div>
      )}
    </section>
  );
}
