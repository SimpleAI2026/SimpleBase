import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { mountWithApp } from '@/test/helpers'
import ModelTraceList from '@/components/models/ModelTraceList.vue'
import TraceDetailModal from '@/components/models/TraceDetailModal.vue'
import TraceIOBlock from '@/components/models/TraceIOBlock.vue'
import type { LLMTraceGroupListResult } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function groupList(groups: LLMTraceGroupListResult['groups'], total = groups.length) {
  return { groups, total }
}

/** 二期（planv6.5 §11.2）：trace 页签为分组列表，一次顶层调用一行。 */
describe('ModelTraceList 分组列表', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('加载并渲染分组行（状态/来源/聚合 tokens/耗时）', async () => {
    api.llmTraces.groups.mockResolvedValue(
      groupList([
        {
          traceId: 'run-1',
          caller: 'agent',
          model: 'gpt-4o-mini',
          provider: 'openai',
          status: 'ok',
          stream: true,
          startedAt: '2026-11-12T06:00:00Z',
          finishedAt: '2026-11-12T06:00:03Z',
          spanCount: 3,
          toolCount: 1,
          llmCount: 2,
          inputTokens: 300,
          outputTokens: 60,
          durationMs: 2000,
          requestId: 'rid-1'
        },
        {
          traceId: 'rid-2',
          caller: 'api',
          model: 'gpt-4o',
          provider: 'openai',
          status: 'error',
          stream: false,
          startedAt: '2026-11-12T07:00:00Z',
          finishedAt: '2026-11-12T07:00:01Z',
          spanCount: 1,
          toolCount: 0,
          llmCount: 1,
          inputTokens: 0,
          outputTokens: 0,
          durationMs: 88,
          errorCode: 'upstream_error',
          requestId: 'rid-2'
        }
      ])
    )
    const { wrapper } = await mountWithApp(ModelTraceList)
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('gpt-4o-mini')
    expect(text).toContain('2000 ms')
    expect(text).toContain('upstream_error')
    expect(text).toContain('失败')
    // 分组接口：默认窗口后端兜底，首次请求带 limit/offset
    expect(api.llmTraces.groups).toHaveBeenCalledWith('testprj1', {
      status: undefined,
      caller: undefined,
      model: undefined,
      limit: 50,
      offset: 0
    })
  })

  it('点击详情按钮 emit openDetail（traceId）', async () => {
    api.llmTraces.groups.mockResolvedValue(
      groupList([
        {
          traceId: 'run-9',
          caller: 'cron',
          model: 'm',
          provider: 'openai',
          status: 'ok',
          stream: false,
          startedAt: '2026-11-12T06:00:00Z',
          finishedAt: '2026-11-12T06:00:01Z',
          spanCount: 1,
          toolCount: 0,
          llmCount: 1,
          inputTokens: 1,
          outputTokens: 1,
          durationMs: 100,
          requestId: 'r'
        }
      ])
    )
    const { wrapper } = await mountWithApp(ModelTraceList)
    await flushPromises()
    const detailBtn = wrapper.findAll('button').find((b) => b.text().includes('详情'))
    expect(detailBtn).toBeTruthy()
    await detailBtn!.trigger('click')
    expect(wrapper.emitted('openDetail')?.[0]).toEqual(['run-9'])
  })

  it('空结果渲染空态而非报错', async () => {
    api.llmTraces.groups.mockResolvedValue(groupList([], 0))
    const { wrapper } = await mountWithApp(ModelTraceList)
    await flushPromises()
    expect(wrapper.text()).toContain('发起一次模型调用后')
  })

  it('接口失败渲染错误态且可重试', async () => {
    api.llmTraces.groups.mockRejectedValueOnce(new Error('network down'))
    const { wrapper } = await mountWithApp(ModelTraceList)
    await flushPromises()
    expect(wrapper.text()).toContain('network down')

    api.llmTraces.groups.mockResolvedValue(groupList([]))
    const retryBtn = wrapper.findAll('button').find((b) => b.text().includes('重试'))
    await retryBtn!.trigger('click')
    await flushPromises()
    expect(api.llmTraces.groups).toHaveBeenCalledTimes(2)
  })
})

describe('TraceDetailModal & TraceIOBlock', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('TraceIOBlock 自适应渲染消息数组、LLM 输出、工具声明、截断标记与原始文本', async () => {
    const msgWrap = await mountWithApp(TraceIOBlock, {
      props: {
        label: '输入消息',
        json: JSON.stringify([
          { role: 'system', content: 'sys' },
          { role: 'user', content: 'hi' },
          { role: 'assistant', content: 'call', tool_calls: [{ function: { name: 'fn', arguments: 'x'.repeat(90) } }] },
          { role: 'tool', content: 'ok' }
        ])
      }
    })
    expect(msgWrap.wrapper.text()).toContain('system')
    expect(msgWrap.wrapper.text()).toContain('sys')

    const outWrap = await mountWithApp(TraceIOBlock, {
      props: {
        label: '输出',
        json: JSON.stringify({ content: 'hello', error: 'warn', toolCalls: [{ name: 'sql', arguments: '{}' }] })
      }
    })
    expect(outWrap.wrapper.text()).toContain('hello')
    expect(outWrap.wrapper.text()).toContain('sql({})')

    const toolsWrap = await mountWithApp(TraceIOBlock, {
      props: {
        label: '工具声明',
        json: JSON.stringify([{ name: 'list_db', description: '列出数据库' }])
      }
    })
    expect(toolsWrap.wrapper.text()).toContain('list_db')
    expect(toolsWrap.wrapper.text()).toContain('列出数据库')

    const truncWrap = await mountWithApp(TraceIOBlock, {
      props: { label: '结果', json: '{"truncated":true}' }
    })
    expect(truncWrap.wrapper.text()).toContain('截断')

    const rawWrap = await mountWithApp(TraceIOBlock, {
      props: { label: '原始', json: 'not-json' }
    })
    expect(rawWrap.wrapper.text()).toContain('not-json')
  })

  it('TraceDetailModal 渲染 llm/tool span 时间线并支持空态、错误重试与关闭', async () => {
    api.llmTraces.detail.mockResolvedValueOnce([
      {
        id: 'sp-1',
        projectId: 'testprj1',
        requestId: 'req-1',
        source: 'llm',
        provider: 'openai',
        model: 'gpt-4o-mini',
        status: 'ok',
        stream: true,
        durationMs: 120,
        inputTokens: 50,
        outputTokens: 20,
        traceId: 'tr-1',
        caller: 'agent',
        spanKind: 'llm',
        name: 'gpt-4o-mini',
        inputJson: JSON.stringify([{ role: 'user', content: 'ping' }]),
        toolsJson: JSON.stringify([{ name: 'search' }]),
        outputJson: JSON.stringify({ content: 'pong' }),
        createdAt: '2026-11-12T06:00:00Z'
      },
      {
        id: 'sp-2',
        projectId: 'testprj1',
        requestId: 'req-1',
        source: 'llm',
        provider: '',
        model: '',
        status: 'error',
        stream: false,
        durationMs: 35,
        inputTokens: 0,
        outputTokens: 0,
        errorMessage: 'tool timeout',
        traceId: 'tr-1',
        caller: 'agent',
        spanKind: 'tool',
        name: 'search_logs',
        inputJson: '{"q":"err"}',
        outputJson: 'failed',
        createdAt: '2026-11-12T06:00:01Z'
      }
    ])
    const { wrapper } = await mountWithApp(TraceDetailModal, {
      props: { traceId: 'tr-1' },
      stubs: { SbModal: { props: ['open', 'title'], emits: ['update:open'], template: '<div v-if="open"><slot /><button class="close-btn" @click="$emit(\'update:open\', false)">close</button></div>' } }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('gpt-4o-mini')
    expect(wrapper.text()).toContain('search_logs')
    expect(wrapper.text()).toContain('输入消息')
    expect(wrapper.text()).toContain('调用参数')
    expect(wrapper.text()).toContain('tool timeout')

    await wrapper.find('.close-btn').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()

    // 切换为空 traceId 重置，再切到失败 trace 并点击重试恢复为空态
    await wrapper.setProps({ traceId: undefined })
    api.llmTraces.detail.mockRejectedValueOnce(new Error('detail boom'))
    await wrapper.setProps({ traceId: 'tr-err' })
    await flushPromises()
    expect(wrapper.text()).toContain('detail boom')

    api.llmTraces.detail.mockResolvedValueOnce([])
    const retry = wrapper.findAll('button').find((b) => b.text().includes('重试'))
    await retry!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('无 span 记录')
  })
})
