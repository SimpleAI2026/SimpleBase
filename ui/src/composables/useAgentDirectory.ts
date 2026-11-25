import { computed, ref } from 'vue'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import type { AgentModuleInfo, CloudAgent } from '@/services/api'
import { i18n, t } from '@/locales'
import { displayAgentName } from '@/utils/agent-names'
import { errorMessage } from '@/utils/format'
import { useLoadState } from '@/composables/useLoadState'

export interface UseAgentDirectoryOptions {
  projectId: () => string
}

/**
 * 助手目录：助手列表 / 能力模块 / 模型候选的加载，以及按当前语言解析的显示名与索引。
 *
 * 内置助手没改过名时按当前语言显示 `pages.agents.builtinNames.*`；用户改过名的原样保留。
 */
export function useAgentDirectory(opts: UseAgentDirectoryOptions) {
  const { pending, error, run } = useLoadState({ fallback: 'pages.agents.loadAgentsFailed' })

  const agents = ref<CloudAgent[]>([])
  const modules = ref<AgentModuleInfo[]>([])
  const defaultModel = ref('')
  const modelOptions = ref<string[]>([])

  const displayedAgents = computed(() => {
    const locale = i18n.global.locale.value
    return agents.value.map((agent) => {
      const name = displayAgentName(agent, locale)
      return name === agent.name ? agent : { ...agent, name }
    })
  })
  const rawAgentById = computed(() => new Map(agents.value.map((a) => [a.id, a])))
  const displayedAgentById = computed(() => new Map(displayedAgents.value.map((a) => [a.id, a])))
  /** agentId → 当前语言显示名（ConversationView 回答者行）。 */
  const agentNamesById = computed(() => {
    const map: Record<string, string> = {}
    for (const a of displayedAgents.value) map[a.id] = a.name
    return map
  })

  async function loadModules() {
    try {
      modules.value = await api.agents.modules(opts.projectId())
    } catch (e) {
      toast.error(errorMessage(e, t('pages.agents.loadModulesFailed')))
    }
  }

  async function loadAgents() {
    const projectId = opts.projectId()
    await run(async () => {
      await loadModules()
      const list = await api.agents.list(projectId)
      if (opts.projectId() !== projectId) return false
      agents.value = list
      return agents.value.length > 0
    })
  }

  async function loadModelOptions() {
    try {
      // 优先用 /agents/models 提供模型候选。
      const res = await api.agents.models(opts.projectId())
      defaultModel.value = res.default_model || ''
      modelOptions.value = res.models.map((m) => m.name)
      if (!modelOptions.value.length && defaultModel.value) modelOptions.value = [defaultModel.value]
    } catch (e) {
      defaultModel.value = ''
      modelOptions.value = []
      toast.error(errorMessage(e, t('pages.agents.loadModelsFailed')))
    }
  }

  return {
    pending,
    error,
    agents,
    modules,
    defaultModel,
    modelOptions,
    displayedAgents,
    rawAgentById,
    displayedAgentById,
    agentNamesById,
    loadModules,
    loadAgents,
    loadModelOptions
  }
}
