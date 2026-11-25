// 工作台只读视图：页头标题、群成员、@ 候选（含「当前人」）、标签编辑候选与最近会话。
import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { t } from '@/locales'
import { useWorkspaceView, tagDisplayName } from '@/composables/useWorkspaceView'
import { BUILTIN_SEED_NAMES } from '@/utils/agent-names'
import type { AgentTag, AgentThread, CloudAgent } from '@/services/types'

function agent(id: string, name: string, builtinKey?: string): CloudAgent {
  return {
    id, name, module: 'chat', description: '', system_prompt: '', tool_ids: [], team_enabled: true,
    created_at: 't', updated_at: 't', builtin_key: builtinKey
  }
}

function tag(id: string, agentIds: string[], extra: Partial<AgentTag> = {}): AgentTag {
  return { id, name: id, agentIds, prompts: [], unavailableAgentIds: [], routingPrompt: '', threadKinds: ['chat'], version: 1, ...extra }
}

function thread(id: string): AgentThread {
  return { id, title: id, tag_id: 'g1', created_at: 't', updated_at: 't' }
}

function setup(tags: AgentTag[], agents: CloudAgent[], options: { hasUser?: boolean; section?: string; threadId?: string; threads?: AgentThread[] } = {}) {
  const section = ref(options.section ?? 'g1')
  const threadId = ref(options.threadId ?? '')
  const displayedAgentById = ref(new Map(agents.map((a) => [a.id, a])))
  const rawAgentById = ref(new Map(agents.map((a) => [a.id, a])))
  const view = useWorkspaceView({
    tags: ref(tags),
    section,
    threadId,
    sectionThreads: ref(options.threads ?? [thread('t1'), thread('t2'), thread('t3'), thread('t4'), thread('t5'), thread('t6')]),
    displayedAgents: ref(agents),
    displayedAgentById,
    rawAgentById,
    hasUser: () => options.hasUser ?? true
  })
  return { view, section, threadId }
}

describe('useWorkspaceView', () => {
  it('页头标题：内置群走 i18n，自定义群用群名，未知分区回落到「对话」', () => {
    const tags = [tag('general', [], { builtinKey: 'general', name: '通用' }), tag('ops', [])]
    const { view, section } = setup(tags, [], { section: 'general' })
    expect(tagDisplayName(tags[0])).toBe(t('components.agentWorkspace.builtinTags.general'))
    expect(view.headerTitle.value).toBe(t('components.agentWorkspace.builtinTags.general'))
    expect(view.sectionDisplayName.value).toBe(t('components.agentWorkspace.builtinTags.general'))

    section.value = 'ops'
    expect(view.headerTitle.value).toBe('ops')

    section.value = 'no-such-tag'
    expect(view.activeTag.value).toBeNull()
    expect(view.headerTitle.value).toBe(t('pages.agents.dialogLabel'))
    expect(view.sectionAgents.value).toEqual([])
  })

  it('@ 候选只列本群成员；登录态且群允许人工时追加「当前人」，API Key 主体或群禁用人工时不追加', () => {
    const tags = [tag('g1', ['a1', 'a2', 'ghost'])]
    const agents = [agent('a1', '甲'), agent('a2', '乙'), agent('a3', '群外')]
    const { view } = setup(tags, agents)
    expect(view.sectionAgents.value.map((a) => a.id)).toEqual(['a1', 'a2'])
    const mention = view.mentionAgents.value
    expect(mention.map((m) => m.id)).toEqual(['a1', 'a2', 'me'])
    expect(mention.at(-1)).toMatchObject({ kind: 'human', name: t('components.agentWorkspace.mentionMe') })

    const apiKey = setup(tags, agents, { hasUser: false }).view
    expect(apiKey.mentionAgents.value.map((m) => m.id)).toEqual(['a1', 'a2'])

    const disabled = setup([tag('g1', ['a1'], { humanPolicy: 'disabled' })], agents).view
    expect(disabled.mentionAgents.value.map((m) => m.id)).toEqual(['a1'])
  })

  it('内置种子助手带 builtinKey 与旧名字，改名或自定义助手不带', () => {
    const seed = agent('a1', BUILTIN_SEED_NAMES.database, 'database')
    const renamed = agent('a2', '小库', 'database')
    const custom = agent('a3', '自定义')
    const { view } = setup([tag('g1', ['a1', 'a2', 'a3'])], [seed, renamed, custom])
    const byId = new Map(view.mentionAgents.value.map((m) => [m.id, m]))
    expect(byId.get('a1')).toMatchObject({ builtinKey: 'database', legacyName: BUILTIN_SEED_NAMES.database })
    expect(byId.get('a2')).toMatchObject({ builtinKey: undefined, legacyName: '小库' })
    expect(byId.get('a3')).toMatchObject({ builtinKey: undefined, legacyName: undefined })
  })

  it('agentOptionsFor：成员之外补上「助手已不可用」的占位，非成员的失效 ID 不补', () => {
    const agents = [agent('a1', '甲'), agent('a2', '乙')]
    const { view } = setup([tag('g1', ['a1', 'gone'], { unavailableAgentIds: ['gone', 'stranger'] })], agents)
    const options = view.agentOptionsFor(view.activeTag.value)
    expect(options.map((o) => o.id)).toEqual(['a1', 'a2', 'gone'])
    expect(options.at(-1)?.name).toBe(t('components.agentTagOverview.unavailable'))
    expect(view.agentOptionsFor(null).map((o) => o.id)).toEqual(['a1', 'a2'])
  })

  it('最近会话只取前 5 条；当前会话按 threadId 解析', () => {
    const { view, threadId } = setup([tag('g1', [])], [])
    expect(view.recentTagThreads.value.map((th) => th.id)).toEqual(['t1', 't2', 't3', 't4', 't5'])
    expect(view.activeThread.value).toBeUndefined()
    threadId.value = 't3'
    expect(view.activeThread.value?.id).toBe('t3')
  })
})
