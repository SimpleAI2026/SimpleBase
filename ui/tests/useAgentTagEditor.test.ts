import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { useAgentTagEditor } from '@/composables/useAgentTagEditor'
import type { AgentTag } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function tag(id: string, version = 1): AgentTag {
  return { id, name: id, agentIds: [], prompts: [], unavailableAgentIds: [], routingPrompt: '', threadKinds: ['chat'], version }
}

function conflict() {
  return Object.assign(new Error('version conflict'), { status: 409 })
}

function setup() {
  const tags = ref<AgentTag[]>([tag('general'), tag('g1', 3)])
  const section = ref('general')
  const loadTags = vi.fn().mockResolvedValue(undefined)
  const loadAgents = vi.fn().mockResolvedValue(undefined)
  const selectSection = vi.fn().mockResolvedValue(undefined)
  const editor = useAgentTagEditor({ projectId: () => 'p1', tags, section, loadTags, loadAgents, selectSection })
  return { tags, section, loadTags, loadAgents, selectSection, editor }
}

describe('useAgentTagEditor：标签', () => {
  beforeEach(() => resetApiMocks())

  it('openTag 无 id 新建、有 id 编辑，找不到的 id 什么都不做', () => {
    const { editor } = setup()
    editor.tagModal.value.conflicted = true
    editor.openTag()
    expect(editor.tagModal.value).toMatchObject({ open: true, conflicted: false })
    expect(editor.editingTag.value).toBeNull()

    editor.tagModal.value.open = false
    editor.openTag('g1')
    expect(editor.editingTag.value?.id).toBe('g1')
    expect(editor.tagModal.value.open).toBe(true)

    editor.tagModal.value.open = false
    editor.openTag('nope')
    expect(editor.tagModal.value.open).toBe(false)
  })

  it('saveTag：新建走 create，编辑走 patch，成功后关弹窗并重载标签', async () => {
    const { editor, loadTags } = setup()
    editor.openTag()
    await editor.saveTag({ name: 'x' } as never)
    expect(api.agentTags.create).toHaveBeenCalledWith('p1', { name: 'x' })
    expect(editor.tagModal.value).toMatchObject({ open: false, saving: false, conflicted: false })
    expect(loadTags).toHaveBeenCalledTimes(1)

    editor.openTag('g1')
    await editor.saveTag({ name: 'y' } as never)
    expect(api.agentTags.patch).toHaveBeenCalledWith('p1', 'g1', { name: 'y' })
  })

  it('saveTag：409 只置 conflicted（保持弹窗打开），其他错误 toast', async () => {
    const { editor, loadTags } = setup()
    editor.openTag('g1')
    api.agentTags.patch.mockRejectedValueOnce(conflict())
    await editor.saveTag({ name: 'y' } as never)
    expect(editor.tagModal.value).toMatchObject({ open: true, conflicted: true, saving: false })
    expect(toast.error).not.toHaveBeenCalled()
    expect(loadTags).not.toHaveBeenCalled()

    api.agentTags.patch.mockRejectedValueOnce(new Error('boom'))
    await editor.saveTag({ name: 'y' } as never)
    expect(toast.error).toHaveBeenCalledWith('boom')
  })

  it('reloadTag 用服务端版本回填并清掉冲突；失败 toast', async () => {
    const { editor, tags } = setup()
    editor.openTag('g1')
    editor.tagModal.value.conflicted = true
    api.agentTags.list.mockResolvedValueOnce([tag('g1', 9)])
    await editor.reloadTag()
    expect(tags.value).toHaveLength(1)
    expect(editor.editingTag.value?.version).toBe(9)
    expect(editor.tagModal.value.conflicted).toBe(false)

    api.agentTags.list.mockRejectedValueOnce(new Error('offline'))
    await editor.reloadTag()
    expect(toast.error).toHaveBeenCalledWith('offline')
  })

  it('archiveTag：归档当前群后回到 #通用，其他群不切换', async () => {
    const { editor, section, selectSection, loadTags } = setup()
    await editor.archiveTag('g1')
    expect(api.agentTags.remove).toHaveBeenCalledWith('p1', 'g1')
    expect(loadTags).toHaveBeenCalled()
    expect(selectSection).not.toHaveBeenCalled()

    section.value = 'g1'
    await editor.archiveTag('g1')
    expect(selectSection).toHaveBeenCalledWith('general')

    api.agentTags.remove.mockRejectedValueOnce(new Error('denied'))
    await editor.archiveTag('g1')
    expect(toast.error).toHaveBeenCalledWith('denied')
  })
})

describe('useAgentTagEditor：群成员', () => {
  beforeEach(() => resetApiMocks())

  it('openMembers 默认「加入已有」，可指定 create；并清掉旧冲突', () => {
    const { editor } = setup()
    editor.membersModal.value.conflicted = true
    editor.openMembers('g1')
    expect(editor.membersMode.value).toBe('existing')
    expect(editor.membersModal.value).toMatchObject({ open: true, conflicted: false })
    expect(editor.membersGroup.value?.id).toBe('g1')

    editor.openMembers('general', 'create')
    expect(editor.membersMode.value).toBe('create')
  })

  it('saveMembers 带群版本做乐观锁；新建助手时刷新助手列表；当前群再拉一次群配置', async () => {
    const { editor, section, loadTags, loadAgents } = setup()
    section.value = 'g1'
    editor.openMembers('g1')
    await editor.saveMembers({ newAgent: { name: 'a', module: 'general' } })
    expect(api.agentTags.addMembers).toHaveBeenCalledWith('p1', 'g1', {
      newAgent: { name: 'a', module: 'general' },
      expectedVersion: 3
    })
    expect(editor.membersModal.value).toMatchObject({ open: false, saving: false })
    expect(loadAgents).toHaveBeenCalledTimes(1)
    expect(loadTags).toHaveBeenCalledTimes(2)
  })

  it('saveMembers：加入已有且非当前群时不刷新助手、不二次拉群', async () => {
    const { editor, loadTags, loadAgents } = setup()
    editor.openMembers('g1')
    await editor.saveMembers({ agentIds: ['a1'] })
    expect(loadAgents).not.toHaveBeenCalled()
    expect(loadTags).toHaveBeenCalledTimes(1)
  })

  it('saveMembers：没选群直接返回；409 置冲突；其他错误 toast', async () => {
    const { editor } = setup()
    await editor.saveMembers({ agentIds: ['a'] })
    expect(api.agentTags.addMembers).not.toHaveBeenCalled()

    editor.openMembers('g1')
    api.agentTags.addMembers.mockRejectedValueOnce(conflict())
    await editor.saveMembers({ agentIds: ['a'] })
    expect(editor.membersModal.value).toMatchObject({ open: true, conflicted: true })

    api.agentTags.addMembers.mockRejectedValueOnce(new Error('nope'))
    await editor.saveMembers({ agentIds: ['a'] })
    expect(toast.error).toHaveBeenCalledWith('nope')
  })

  it('reloadMembers 重载群并清掉冲突', async () => {
    const { editor, loadTags } = setup()
    editor.membersModal.value.conflicted = true
    await editor.reloadMembers()
    expect(loadTags).toHaveBeenCalled()
    expect(editor.membersModal.value.conflicted).toBe(false)
  })
})
