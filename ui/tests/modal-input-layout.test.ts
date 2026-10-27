import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 弹窗内「输入框 + 其他元素」并排行的布局回归：
 * 并排行必须 flex-wrap，主输入框必须 min-w-0，固定宽度只能是 `sm:` 之后或 shrink-0 的短框，
 * 否则窄视口会把输入框撑出 modal，焦点环也会被 body 的 overflow-x-hidden 裁掉。
 * 这些都是纯 class 约定，直接按源码断言，避免为布局挂载一整套桩。
 */
function read(rel: string): string {
  return readFileSync(resolve(__dirname, '../src/components', rel), 'utf-8')
}

describe('modal input layout', () => {
  it('SbModal body keeps ring room via px-1 -mx-1', () => {
    const src = read('modal/SbModal.vue')
    expect(src).toMatch(/data-slot="sb-modal-body" class="[^"]*-mx-1[^"]*px-1/)
  })

  it('GoFunctionModal name row wraps and input shrinks', () => {
    const src = read('modal/GoFunctionModal.vue')
    expect(src).toContain('flex flex-wrap items-center gap-2')
    expect(src).toContain('min-w-0 flex-1 sm:w-56 sm:flex-none')
    expect(src).not.toMatch(/class="w-56 sb-mono"/)
  })

  it('SqlWorkModal params inputs are flexible', () => {
    const src = read('modal/SqlWorkModal.vue')
    expect(src.match(/min-w-0 flex-1 sm:max-w-70/g)).toHaveLength(2)
    expect(src).not.toMatch(/class="max-w-70"/)
    expect(src).toContain('w-30 shrink-0')
  })

  it('KvCreateKeyModal pair rows and ttl row wrap', () => {
    const src = read('databases/kv/KvCreateKeyModal.vue')
    expect(src.match(/flex flex-wrap items-center gap-2/g)?.length).toBeGreaterThanOrEqual(3)
    expect(src).toContain('w-28 shrink-0')
    expect(src).toContain('min-w-32 flex-1 sm:w-40 sm:flex-none')
    expect(src).not.toMatch(/class="w-40"/)
  })

  it('KvTtlModal strategy row wraps', () => {
    const src = read('databases/kv/KvTtlModal.vue')
    expect(src).toContain('flex flex-wrap items-center gap-2')
    expect(src).toContain('min-w-32 flex-1 sm:w-40 sm:flex-none')
    expect(src).not.toMatch(/class="w-40" /)
  })

  it('KvKeyListPanel search input shrinks on narrow width', () => {
    const src = read('databases/kv/KvKeyListPanel.vue')
    expect(src).toContain('min-w-0 flex-1 sm:w-64 sm:flex-none')
    expect(src).toContain('w-28 shrink-0 sm:w-32')
  })

  it.each([
    ['databases/kv/editors/KvSetEditor.vue', 'min-w-0 flex-1 sm:w-48 sm:flex-none'],
    ['databases/kv/editors/KvListEditor.vue', 'min-w-0 flex-1 sm:w-48 sm:flex-none'],
    ['databases/kv/editors/KvHashEditor.vue', 'min-w-32 flex-1 sm:w-36 sm:flex-none'],
    ['databases/kv/editors/KvZSetEditor.vue', 'min-w-32 flex-1 sm:w-36 sm:flex-none']
  ])('%s add-row input is responsive', (file, cls) => {
    const src = read(file)
    expect(src).toContain(cls)
    expect(src).not.toMatch(/class="w-(36|48)"/)
  })

  it('Kv hash / zset add rows wrap', () => {
    for (const f of ['databases/kv/editors/KvHashEditor.vue', 'databases/kv/editors/KvZSetEditor.vue']) {
      const src = read(f)
      expect(src).toContain('v-if="!readonly" class="flex flex-wrap items-center gap-2"')
      expect(src).not.toContain('flex items-end gap-2')
    }
  })

  it('AgentScheduleModal frequency row wraps without a dead min-w-50 on Select', () => {
    const src = read('ai/AgentScheduleModal.vue')
    expect(src).toContain('flex flex-wrap items-center justify-between')
    expect(src).not.toContain('class="min-w-50"')
  })

  it('FunctionTriggerModal uses responsive fields without nested horizontal scrolling', () => {
    const src = read('modal/FunctionTriggerModal.vue')
    expect(src).toContain('grid gap-5 md:grid-cols-2')
    expect(src).toContain('class="md:col-span-2"')
    expect(src).not.toContain('overflow-x-auto')
    expect(src).not.toContain('class="w-56"')
  })
})
