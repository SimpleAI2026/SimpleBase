import { watch, type Ref } from 'vue'
import { api } from '@/services/api'
import { useProjectStore } from '@/stores/project'

/** API 预览参数下拉的一个候选项：`value` 写入参数，`label` 展示名称，`hint` 展示 id。 */
export interface ParamOption {
  value: string
  label: string
  hint?: string
}

type Loader = (projectId: string, deps: Record<string, string>) => Promise<ParamOption[]>

function byId(id: string, name: string): ParamOption {
  return { value: id, label: name || id, hint: name && name !== id ? id : undefined }
}

const LOADERS: Record<string, Loader> = {
  databases: async (pid) => (await api.databases.list(pid)).map((d) => byId(d.id, d.name)),
  collections: async (pid, deps) => (await api.db.collections(pid, deps.databaseID)).map((c) => ({ value: c, label: c })),
  tables: async (pid, deps) => (await api.databases.schema(pid, deps.databaseID)).tables.map((x) => ({ value: x.name, label: x.name })),
  agents: async (pid) => (await api.agents.list(pid)).map((a) => byId(a.id, a.name)),
  'agent-threads': async (pid) => (await api.agentThreads.list(pid)).map((x) => byId(x.id, x.title)),
  'cron-jobs': async (pid) => (await api.cronjobs.list(pid)).map((j) => byId(j.id, j.name)),
  gofunctions: async (pid) => (await api.gofunctions.list(pid)).map((f) => ({ value: f.name, label: f.name })),
  'function-triggers': async (pid) => (await api.triggers.list(pid)).map((x) => byId(x.id, x.target)),
  sandboxes: async (pid) => (await api.sandboxes.list(pid)).map((s) => byId(s.id, s.name)),
  'mcp-servers': async (pid) => (await api.mcpServers.list(pid)).servers.map((s) => byId(s.id, s.name)),
  'api-keys': async (pid) => (await api.apiKeys.list(pid)).map((k) => ({ value: k.id, label: k.id })),
  backups: async (pid) => (await api.backups.list(pid)).map((b) => ({ value: b.id, label: b.id, hint: b.createdAt }))
}

/**
 * API 预览的参数候选项。数据源全部复用现有 `api.*`，不新增后端接口。
 * 结果按 `项目 + kind + 依赖值` 缓存，项目切换时清空；失败不缓存，由调用方退化为自由输入。
 */
export function useApiParamOptions(projectId: Ref<string>) {
  const projectStore = useProjectStore()
  const cache = new Map<string, Promise<ParamOption[]>>()

  watch(projectId, () => cache.clear())

  function cacheKey(kind: string, deps: Record<string, string>): string {
    const depPart = Object.keys(deps)
      .sort()
      .map((k) => `${k}=${deps[k]}`)
      .join('&')
    return `${projectId.value}|${kind}|${depPart}`
  }

  /** 候选项；未知 kind 返回空数组，数据源失败则 reject。 */
  function options(kind: string, deps: Record<string, string> = {}): Promise<ParamOption[]> {
    if (kind === 'project') {
      return Promise.resolve(projectStore.projects.map((p) => byId(p.id, p.name)))
    }
    const loader = LOADERS[kind]
    if (!loader) return Promise.resolve([])
    const key = cacheKey(kind, deps)
    const hit = cache.get(key)
    if (hit) return hit
    const pending = loader(projectId.value, deps)
    cache.set(key, pending)
    pending.catch(() => {
      if (cache.get(key) === pending) cache.delete(key)
    })
    return pending
  }

  return { options }
}
