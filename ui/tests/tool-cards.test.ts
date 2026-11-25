import { describe, expect, it } from 'vitest'
import {
  applyToolCall,
  applyToolConfirmation,
  applyToolDelta,
  applyToolResult,
  applyToolStart
} from '@/utils/tool-cards'
import type { AgentToolCallCard } from '@/services/types'

const card = (over: Partial<AgentToolCallCard>): AgentToolCallCard => ({ name: 'list_databases', ...over })

describe('tool-cards：参数增量', () => {
  it('同 call_id 追加参数；首次出现则新增 generating 卡', () => {
    let cards = applyToolDelta([], 'sql', '{"a"', 'c1')
    expect(cards).toEqual([{ call_id: 'c1', name: 'sql', arguments: '{"a"', status: 'generating' }])
    cards = applyToolDelta(cards, 'sql', ':1}', 'c1')
    expect(cards).toHaveLength(1)
    expect(cards[0].arguments).toBe('{"a":1}')
  })

  it('卡片已进入 pending / running / success 后不再追加参数，且原样返回同一个数组', () => {
    for (const status of ['pending', 'running', 'success', 'error'] as const) {
      const input = [card({ call_id: 'c1', arguments: 'x', status })]
      const out = applyToolDelta(input, 'n', 'LATE', 'c1')
      expect(out, status).toBe(input)
      expect(out[0].arguments, status).toBe('x')
      expect(out[0].status, status).toBe(status)
    }
  })

  it('无状态的卡视为生成中；无 call_id 的 delta 不与已有卡合并', () => {
    expect(applyToolDelta([card({ call_id: 'c1', arguments: 'a' })], 'n', 'b', 'c1')[0]).toMatchObject({ arguments: 'ab', status: 'generating' })
    const cards = applyToolDelta([card({ call_id: 'c1', arguments: 'a', status: 'generating' })], 'n', 'b')
    expect(cards).toHaveLength(2)
  })

  it('返回新数组，入参数组长度不变', () => {
    const input = [card({ call_id: 'c1', status: 'generating', arguments: '' })]
    const out = applyToolDelta(input, 'n', 'x', 'c1')
    expect(out).not.toBe(input)
    expect(input).toHaveLength(1)
    expect(applyToolDelta(input, 'n', 'x', 'c2')).toHaveLength(2)
    expect(input).toHaveLength(1)
  })
})

describe('tool-cards：完整调用 / 开始执行', () => {
  it('applyToolCall：生成中 → pending 且参数以完整调用为准；已 running 的卡状态不回退', () => {
    expect(applyToolCall([card({ call_id: 'c1', arguments: 'part', status: 'generating' })], 'sql', 'FULL', 'c1')[0])
      .toMatchObject({ name: 'sql', arguments: 'FULL', status: 'pending' })
    expect(applyToolCall([card({ call_id: 'c1', status: 'running' })], 'sql', 'FULL', 'c1')[0].status).toBe('running')
    expect(applyToolCall([], 'sql', 'A', 'c9')).toEqual([{ call_id: 'c9', name: 'sql', arguments: 'A', status: 'pending' }])
  })

  it('applyToolStart：有 id 精确匹配；无 id 按同名取最后一张；都没有则新增', () => {
    const base = [card({ call_id: 'c1', status: 'pending' }), card({ call_id: 'c2', status: 'pending' })]
    expect(applyToolStart(base, 'list_databases', 'c1').map((c) => c.status)).toEqual(['running', 'pending'])
    const byName = applyToolStart([card({ call_id: 'a' }), card({ call_id: 'b' })], 'list_databases')
    expect(byName.map((c) => c.status)).toEqual([undefined, 'running'])
    expect(applyToolStart([], 'x', 'c3')).toEqual([{ call_id: 'c3', name: 'x', status: 'running' }])
    // 有 id 但没匹配上：不降级到同名匹配，直接新增。
    expect(applyToolStart([card({ call_id: 'c1' })], 'list_databases', 'zzz')).toHaveLength(2)
  })
})

describe('tool-cards：执行结果', () => {
  it('乱序到达：按 call_id 各自落到自己的卡', () => {
    let cards = [card({ call_id: 'c1', status: 'running' }), card({ call_id: 'c2', status: 'running' })]
    cards = applyToolResult(cards, 'list_databases', { content: 'second', durationMs: 20 }, 'c2')
    cards = applyToolResult(cards, 'list_databases', { content: 'first', durationMs: 10 }, 'c1')
    expect(cards.map((c) => c.content)).toEqual(['first', 'second'])
    expect(cards.every((c) => c.status === 'success')).toBe(true)
  })

  it('无 call_id 的结果：落到最后一张同名且尚无结果的卡；没有则新增', () => {
    let cards = [card({ call_id: 'c1', status: 'running' }), card({ call_id: 'c2', status: 'running' })]
    cards = applyToolResult(cards, 'list_databases', { content: 'A' })
    expect(cards.map((c) => c.content)).toEqual([undefined, 'A'])
    cards = applyToolResult(cards, 'list_databases', { content: 'B' })
    expect(cards.map((c) => c.content)).toEqual(['B', 'A'])
    cards = applyToolResult(cards, 'list_databases', { content: 'C' })
    expect(cards).toHaveLength(3)
    expect(applyToolResult([], 'other', { content: 'X' })[0]).toMatchObject({ name: 'other', content: 'X', status: 'success' })
  })

  it('错误 / 截断标记与耗时', () => {
    const cards = applyToolResult([card({ call_id: 'c1', status: 'running' })], 'n', { content: 'bad', durationMs: 5, isError: true, truncated: true }, 'c1')
    expect(cards[0]).toMatchObject({ status: 'error', is_error: true, truncated: true, duration_ms: 5, content: 'bad' })
    const added = applyToolResult([], 'n', { content: 'x' }, 'zz')
    expect(added[0]).toMatchObject({ is_error: false, truncated: false, status: 'success' })
  })
})

describe('tool-cards：等待确认', () => {
  it('只按 call_id 精确匹配；找不到就原样返回同一个数组', () => {
    const cards = [card({ call_id: 'c1', status: 'running' })]
    expect(applyToolConfirmation(cards, 'nope', ['x'])).toBe(cards)
    const out = applyToolConfirmation(cards, 'c1', ['rm', '-rf'])
    expect(out[0]).toMatchObject({ status: 'awaiting_confirm', argv: ['rm', '-rf'] })
  })
})
