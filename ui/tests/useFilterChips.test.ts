import { describe, expect, it } from 'vitest'
import { useFilterChips } from '@/composables/useFilterChips'

const chip = (id: string, expr = `${id}="x"`) => ({ id, label: id, expr })

describe('useFilterChips', () => {
  it('apply：草稿去空格后生效并进历史；空草稿不入历史', () => {
    const f = useFilterChips()
    f.draft.value = '  status="paid"  '
    f.apply()
    expect(f.applied.value).toBe('status="paid"')
    expect(f.history.value).toEqual(['status="paid"'])
    f.draft.value = '   '
    f.apply()
    expect(f.applied.value).toBe('')
    expect(f.history.value).toEqual(['status="paid"'])
  })

  it('appendField：空草稿直接写名字，已有内容用 && 连接，以空格结尾则不加', () => {
    const f = useFilterChips()
    f.appendField('amount')
    expect(f.draft.value).toBe('amount')
    f.appendField('status')
    expect(f.draft.value).toBe('amount && status')
    f.draft.value = 'amount > 1 '
    f.appendField('qty')
    expect(f.draft.value).toBe('amount > 1 qty')
  })

  it('useHistory：回填并立即应用', () => {
    const f = useFilterChips()
    f.useHistory('a=1')
    expect(f.draft.value).toBe('a=1')
    expect(f.applied.value).toBe('a=1')
  })

  it('addChip 按 id 去重，removeChip 按 id 移除', () => {
    const f = useFilterChips()
    f.addChip(chip('c1'))
    f.addChip(chip('c1', 'other'))
    f.addChip(chip('c2'))
    expect(f.chips.value.map((c) => c.id)).toEqual(['c1', 'c2'])
    expect(f.chips.value[0].expr).toBe('c1="x"')
    f.removeChip('c1')
    expect(f.chips.value.map((c) => c.id)).toEqual(['c2'])
    f.removeChip('nope')
    expect(f.chips.value).toHaveLength(1)
  })

  it('clear 保留历史；reset（换数据源）连历史一起清', () => {
    const f = useFilterChips()
    f.draft.value = 'a=1'
    f.apply()
    f.addChip(chip('c1'))
    f.clear()
    expect([f.draft.value, f.applied.value, f.chips.value.length]).toEqual(['', '', 0])
    expect(f.history.value).toEqual(['a=1'])
    f.addChip(chip('c2'))
    f.reset()
    expect(f.history.value).toEqual([])
    expect(f.chips.value).toEqual([])
  })
})
