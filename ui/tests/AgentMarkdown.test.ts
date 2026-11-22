import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentMarkdown from '@/components/agent/AgentMarkdown.vue'

describe('AgentMarkdown JSON', () => {
  it('formats a complete JSON response as a code block', () => {
    const wrapper = mount(AgentMarkdown, {
      props: { content: '[{"id":"plan","goal":"build"},{"id":"kv","skip_reason":"unused"}]' }
    })
    const code = wrapper.get('pre code')
    expect(code.attributes('class')).toContain('language-json')
    expect(code.text()).toContain('[\n  {\n    "id": "plan",\n    "goal": "build"\n  }')
  })

  it('leaves partial streaming JSON and ordinary Markdown unchanged', async () => {
    const wrapper = mount(AgentMarkdown, { props: { content: '[{"id":"plan"' } })
    expect(wrapper.find('pre').exists()).toBe(false)
    expect(wrapper.text()).toContain('[{"id":"plan"')
    await wrapper.setProps({ content: '**done**' })
    expect(wrapper.find('strong').text()).toBe('done')
  })
})
