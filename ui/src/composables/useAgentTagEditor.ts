import { computed, ref } from 'vue'
import type { Ref } from 'vue'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import { t } from '@/locales'
import type { AgentGroupMembersInput, AgentTag, AgentTagInput } from '@/services/types'
import { isConflict } from '@/utils/api-errors'
import { errorMessage } from '@/utils/format'

/** 弹窗的公共状态：开关 + 保存中 + 版本冲突（乐观锁）。 */
export interface ModalState {
  open: boolean
  saving: boolean
  conflicted: boolean
}

export interface UseAgentTagEditorOptions {
  projectId: () => string
  tags: Ref<AgentTag[]>
  /** 当前所在群：归档当前群后需要回到 #通用。 */
  section: Ref<string>
  loadTags: () => Promise<void>
  loadAgents: () => Promise<void>
  selectSection: (id: string) => Promise<void>
}

/**
 * 标签（群）与群成员的编辑：弹窗状态 + 保存 / 冲突重载 / 归档。
 * 版本冲突（409）只置 conflicted，由弹窗提示「重新加载」；其他错误走 toast。
 */
export function useAgentTagEditor(opts: UseAgentTagEditorOptions) {
  const tagModal = ref<ModalState>({ open: false, saving: false, conflicted: false })
  const editingTag = ref<AgentTag | null>(null)

  const membersModal = ref<ModalState>({ open: false, saving: false, conflicted: false })
  const membersGroupId = ref('')
  const membersMode = ref<'existing' | 'create'>('existing')

  const membersGroup = computed(() => opts.tags.value.find((tag) => tag.id === membersGroupId.value) || null)

  function openTag(id?: string) {
    if (id) {
      const tag = opts.tags.value.find((item) => item.id === id)
      if (!tag) return
      editingTag.value = tag
    } else {
      editingTag.value = null
    }
    tagModal.value.conflicted = false
    tagModal.value.open = true
  }

  async function saveTag(input: AgentTagInput) {
    const projectId = opts.projectId()
    const editing = editingTag.value
    tagModal.value.saving = true
    try {
      if (editing) await api.agentTags.patch(projectId, editing.id, input)
      else await api.agentTags.create(projectId, input)
      tagModal.value.open = false
      tagModal.value.conflicted = false
      await opts.loadTags()
    } catch (e) {
      if (isConflict(e)) tagModal.value.conflicted = true
      else toast.error(errorMessage(e, t('pages.agents.saveFailed')))
    } finally {
      tagModal.value.saving = false
    }
  }

  /** 版本冲突后「重新加载」：丢弃本地修改，用服务端版本回填。 */
  async function reloadTag() {
    const editing = editingTag.value
    if (!editing) return
    try {
      const list = await api.agentTags.list(opts.projectId())
      opts.tags.value = Array.isArray(list) ? list : []
      editingTag.value = list.find((item) => item.id === editing.id) || null
      tagModal.value.conflicted = false
    } catch (e) {
      toast.error(errorMessage(e, t('pages.agents.loadThreadsFailed')))
    }
  }

  async function archiveTag(id: string) {
    try {
      await api.agentTags.remove(opts.projectId(), id)
      await opts.loadTags()
      if (opts.section.value === id) await opts.selectSection('general')
    } catch (e) {
      toast.error(errorMessage(e, t('pages.agents.deleteFailed')))
    }
  }

  /** 打开成员弹窗：默认「加入已有助手」，`create` 直接落在「新建并加入」页签。 */
  function openMembers(groupId: string, mode: 'existing' | 'create' = 'existing') {
    membersGroupId.value = groupId
    membersMode.value = mode
    membersModal.value.conflicted = false
    membersModal.value.open = true
  }

  /** 提交成员变更：加入已有 或 新建并加入（服务端组合事务）。 */
  async function saveMembers(input: AgentGroupMembersInput) {
    const groupId = membersGroupId.value
    if (!groupId) return
    membersModal.value.saving = true
    try {
      await api.agentTags.addMembers(opts.projectId(), groupId, {
        ...input,
        expectedVersion: membersGroup.value?.version
      })
      membersModal.value.open = false
      membersModal.value.conflicted = false
      await opts.loadTags()
      // 新建助手的场景下助手列表也变了：刷新一次，否则 @ 候选里没有它。
      if (input.newAgent) await opts.loadAgents()
      // 当前群的成员变化：再拉一次群配置，候选与身份名都依赖它。
      if (opts.section.value === groupId) await opts.loadTags()
    } catch (e) {
      if (isConflict(e)) membersModal.value.conflicted = true
      else toast.error(errorMessage(e, t('pages.agents.saveFailed')))
    } finally {
      membersModal.value.saving = false
    }
  }

  /** 版本冲突后重新加载群（丢弃本地选择）。 */
  async function reloadMembers() {
    await opts.loadTags()
    membersModal.value.conflicted = false
  }

  return {
    tagModal,
    editingTag,
    membersModal,
    membersMode,
    membersGroup,
    openTag,
    saveTag,
    reloadTag,
    archiveTag,
    openMembers,
    saveMembers,
    reloadMembers
  }
}
