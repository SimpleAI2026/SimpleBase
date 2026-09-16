import { defineStore } from 'pinia'

const STORAGE_KEY = 'sb_project_id'
const HISTORY_KEY = 'sb_project_id_history'
const DEFAULT_PROJECT_ID = 'proj-01'
const HISTORY_MAX = 20

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

/** 当前项目 store：支持后端列表 + 本地历史，缺省 DevMode 种子 proj-01。 */
export const useProjectStore = defineStore('project', {
  state: () => ({
    projectId: localStorage.getItem(STORAGE_KEY) || DEFAULT_PROJECT_ID,
    /** 最近用过的项目 ID（本地） */
    history: loadHistory() as string[]
  }),
  getters: {
    id(): string {
      return this.projectId.trim() || DEFAULT_PROJECT_ID
    }
  },
  actions: {
    setProject(id: string) {
      const v = id.trim()
      if (!v) return
      this.projectId = v
      localStorage.setItem(STORAGE_KEY, v)
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
    }
  }
})
