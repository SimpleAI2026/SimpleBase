import { defineStore } from 'pinia'
import { api } from '../services/api'
import type { ProjectItem } from '../services/types'

const STORAGE_KEY = 'sb_project_id'
const HISTORY_KEY = 'sb_project_id_history'
const HISTORY_MAX = 20
const LEGACY_DEFAULT_ID = 'proj-01'

/** DevMode 种子项目（internal/catalog.DevProjectID / systemdb seed）。 */
export const DEFAULT_PROJECT_ID = '00000000-0000-0000-0000-000000000002'

/** admin（系统）项目：internal/catalog.ReservedSystemProjectID。 */
export const ADMIN_PROJECT_ID = '00000000-0000-0000-0000-000000000099'

function loadHistory(): string[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    const arr = raw ? JSON.parse(raw) : []
    return Array.isArray(arr) ? arr.map(String).filter(Boolean) : []
  } catch {
    return []
  }
}

function saveHistory(ids: string[]) {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(ids.slice(0, HISTORY_MAX)))
}

function initialProjectId(): string {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (!stored || stored === LEGACY_DEFAULT_ID) {
    localStorage.setItem(STORAGE_KEY, DEFAULT_PROJECT_ID)
    return DEFAULT_PROJECT_ID
  }
  return stored
}

/** 当前项目 store：GET /v1/projects 缓存 + 本地历史；缺省 DevMode 种子 UUID。 */
export const useProjectStore = defineStore('project', {
  state: () => ({
    projectId: initialProjectId(),
    projectName: '' as string,
    projects: [] as ProjectItem[],
    history: loadHistory() as string[],
    loading: false,
    createModalOpen: false
  }),
  getters: {
    id(): string {
      return this.projectId.trim()
    },
    current(): ProjectItem | undefined {
      return this.projects.find((p) => p.id === this.id)
    },
    displayName(): string {
      if (this.projectName.trim()) return this.projectName.trim()
      const hit = this.projects.find((p) => p.id === this.id)
      return hit?.name || this.id || '选择项目'
    },
    /** 当前是否为 admin（系统）项目：数据库列表展示系统库且禁止新建/删除。 */
    isAdmin(): boolean {
      return this.id === ADMIN_PROJECT_ID
    }
  },
  actions: {
    setProject(id: string, name?: string) {
      const v = id.trim()
      if (!v) return
      this.projectId = v
      localStorage.setItem(STORAGE_KEY, v)
      if (name !== undefined) {
        this.projectName = name
      } else {
        const hit = this.projects.find((p) => p.id === v)
        this.projectName = hit?.name || ''
      }
      const next = [v, ...this.history.filter((x) => x !== v)]
      this.history = next.slice(0, HISTORY_MAX)
      saveHistory(this.history)
    },
    remember(id: string) {
      const v = id.trim()
      if (!v) return
      const next = [v, ...this.history.filter((x) => x !== v)]
      this.history = next.slice(0, HISTORY_MAX)
      saveHistory(this.history)
    },
    async loadProjects() {
      this.loading = true
      try {
        const list = await api.projects.list()
        this.projects = Array.isArray(list) ? list : []
        for (const p of this.projects) {
          if (p.id) this.remember(p.id)
        }
        const hit = this.projects.find((p) => p.id === this.id)
        if (hit) this.projectName = hit.name || ''
      } catch {
        this.projects = []
      } finally {
        this.loading = false
      }
    },
    openCreateModal() {
      this.createModalOpen = true
    },
    closeCreateModal() {
      this.createModalOpen = false
    }
  }
})
