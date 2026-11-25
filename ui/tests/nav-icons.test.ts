import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
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

  it('renders the module icon on the message avatar', () => {
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
