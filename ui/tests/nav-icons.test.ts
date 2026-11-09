import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentCard from '@/components/agent/AgentCard.vue'
import ConversationView from '@/components/agent/ConversationView.vue'
import { iconForAgentModule, navKeyForAgentModule, sidebarIcons } from '@/components/nav-icons'

const modules = ['database', 'kv', 's3', 'gofunction', 'cron', 'sandbox', 'logs', 'users'] as const
const navKeys = ['databases', 'key-value', 's3', 'gofunctions', 'cron-jobs', 'sandboxes', 'logs', 'users'] as const

describe('agent module icons match the sidebar', () => {
  it('maps each module to the same component the nav uses', () => {
    modules.forEach((module, i) => {
      const key = navKeys[i]
      expect(navKeyForAgentModule(module)).toBe(key)
      expect(iconForAgentModule(module)).toBe(sidebarIcons[key])
      expect(iconForAgentModule(module.toUpperCase())).toBe(sidebarIcons[key])
    })
    expect(navKeyForAgentModule('general')).toBe('agents')
    expect(navKeyForAgentModule('project')).toBe('agents')
    expect(navKeyForAgentModule('')).toBe('agents')
    expect(navKeyForAgentModule(undefined)).toBe('agents')
    expect(iconForAgentModule('no-such-module')).toBe(sidebarIcons.agents)
    expect(iconForAgentModule('general')).toBe(sidebarIcons.agents)
  })

  it('renders the database icon on the agent card and the message avatar', () => {
    const card = mount(AgentCard, {
      props: {
        agent: {
          id: 'ag', name: '数据库', module: 'database', description: '',
          system_prompt: '', tool_ids: [], team_enabled: false, created_at: 't', updated_at: 't'
        }
      }
    })
    expect(card.find('[data-agent-icon="databases"]').exists()).toBe(true)

    const general = mount(AgentCard, {
      props: {
        agent: {
          id: 'g', name: '通用助手', module: 'general', description: '',
          system_prompt: '', tool_ids: [], team_enabled: false, created_at: 't', updated_at: 't'
        }
      }
    })
    expect(general.find('[data-agent-icon="agents"]').exists()).toBe(true)

    const view = mount(ConversationView, {
      props: {
        messages: [
          { role: 'user', content: '建库' },
          { role: 'assistant', content: '好的', module: 'gofunction' },
          { role: 'assistant', content: '通用回复' }
        ]
      }
    })
    expect(view.find('[data-agent-icon="gofunctions"]').exists()).toBe(true)
    expect(view.find('[data-agent-icon="agents"]').exists()).toBe(true)
  })
})
