import { ref } from 'vue'
import { toast } from 'vue-sonner'
import { useRouter } from 'vue-router'
import { api } from '@/services/api'
import { t } from '@/locales'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { findStarterTemplate } from '@/constants/starter-prompts'
import { isConflict } from '@/utils/api-errors'
import { errorMessage } from '@/utils/format'
import { genDnsSafeId } from '@/utils/project-id'
import { clearPendingStarter, savePendingStarter } from '@/utils/starter-storage'

export interface StarterLaunchOptions {
  name: string
  engine: 'duckdb' | 'sqlite'
}

/** 项目 ID 冲突（409）时最多换 ID 重试的次数。 */
export const MAX_ID_RETRIES = 3

/**
 * useStarterLaunch：首页「一句话建应用」启动编排。
 *
 * 1. 未登录 → 打开登录框并记 pendingStarter，登录成功后由 Home 续跑同一张卡片；
 * 2. 登录但没有建项目权限（admin 只读）→ 提示并终止；
 * 3. 生成 DNS 安全 ID → 创建项目，409 换 ID 重试（≤3）；
 * 4. 选中新项目 → 跳转 /console/agents?starter=<id>，由助手页接手。
 *
 * 失败不吞：每个失败分支都给出可见提示，不会留下「点了没反应」。
 */
export function useStarterLaunch() {
  const router = useRouter()
  const auth = useAuthStore()
  const project = useProjectStore()
  const launching = ref(false)

  function loggedIn(): boolean {
    // 登录态或旧的 API Key 通道都算已认证。
    return auth.isAuthenticated || Boolean(auth.apiKey)
  }

  async function launch(templateId: string, opts: StarterLaunchOptions): Promise<boolean> {
    const template = findStarterTemplate(templateId)
    if (!template) {
      toast.error(t('components.starter.unknownTemplate'))
      return false
    }
    const name = opts.name.trim()
    if (!name) {
      toast.warning(t('components.createProject.nameRequired'))
      return false
    }
    if (!loggedIn()) {
      // 不传 reason：非空 loginReason 会渲染成红色「重新登录」警示，这里只是正常的登录引导。
      savePendingStarter({ templateId, name, engine: opts.engine })
      auth.openLogin()
      return false
    }
    if (!auth.canCreateProject) {
      toast.error(t('components.starter.noCreatePermission'))
      return false
    }
    if (launching.value) return false
    launching.value = true
    try {
      let created = null as Awaited<ReturnType<typeof api.projects.create>> | null
      let lastError: unknown = null
      for (let attempt = 0; attempt <= MAX_ID_RETRIES && !created; attempt++) {
        try {
          created = await api.projects.create({ name, id: genDnsSafeId(), engine: opts.engine })
        } catch (e) {
          lastError = e
          // 只有 ID 冲突才换 ID 重试；其他错误（权限、配额、校验）重试没有意义。
          if (!isConflict(e)) break
        }
      }
      if (!created) {
        toast.error(errorMessage(lastError, t('components.starter.createFailed')))
        return false
      }
      clearPendingStarter()
      project.setProject(created.id, created.name || name)
      void project.loadProjects()
      await router.push({ name: 'agents', query: { starter: template.id } })
      return true
    } finally {
      launching.value = false
    }
  }

  return { launching, launch, loggedIn }
}
