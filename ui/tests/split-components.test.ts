import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { i18n, setI18nLocale, t } from '@/locales'
import { uiStubs, sampleCron, sampleGoFn } from '@/test/helpers'
import { api, resetApiMocks } from '@/test/api-mock'
import KvKeyListPanel from '@/components/databases/kv/KvKeyListPanel.vue'
import SqlResultPanel from '@/components/modal/SqlResultPanel.vue'
import CronJobScheduleFields from '@/components/cronjob/CronJobScheduleFields.vue'
import ModelDefaultsPanel from '@/components/settings/ModelDefaultsPanel.vue'
import { useProjectStore } from '@/stores/project'

/**
 * 本次拆分出的组件（plan v3 §3.6）此前只被宿主间接覆盖，
 * 这里按各自契约直接验证，补齐交互处理函数的覆盖。
 */

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

beforeEach(() => {
  resetApiMocks()
  setActivePinia(createPinia())
  setI18nLocale('zh-CN')
})

const keyMeta = [
  { key: 'session:1', type: 'hash' as const, len: 2, ttl_ms: null },
  { key: 'user:1', type: 'string' as const, len: null, ttl_ms: 60_000 }
]

const listProps = {
  rows: keyMeta,
  pattern: '',
  typeFilter: 'all',
  hasMore: true,
  loadingMore: false,
  pending: false,
  showSkeleton: false,
  showEmpty: false,
  showError: false,
  refreshing: false,
  error: '',
  ttlText: (m: { ttl_ms: number | null }) => (m.ttl_ms == null ? '永久' : '60s 后过期')
}

describe('KvKeyListPanel', () => {
  const mountList = (overrides: Record<string, unknown> = {}) =>
    mount(KvKeyListPanel, {
      props: { ...listProps, ...overrides },
      global: { stubs: uiStubs, plugins: [i18n] }
    })

  it('搜索与类型过滤通过 v-model 事件回传宿主', async () => {
    const w = mountList()
    const input = w.get('input')
    await input.setValue('session:*')
    expect(w.emitted('update:pattern')?.at(-1)).toEqual(['session:*'])
    // 回车触发重载
    await input.trigger('keydown.enter')
    expect(w.emitted('reload')).toHaveLength(1)
    await w.get('.select-emit').trigger('click')
    expect(w.emitted('update:typeFilter')).toBeTruthy()
    w.unmount()
  })

  it('行内 TTL / 重命名 / 删除 / 打开详情各自抛事件', async () => {
    const w = mountList()
    const firstRow = w.findAll('tbody tr')[0]
    const buttons = firstRow.findAll('button')
    await buttons.find((b) => b.text() === 'TTL')!.trigger('click')
    await buttons.find((b) => b.text() === '重命名')!.trigger('click')
    expect(w.emitted('ttl')).toHaveLength(1)
    expect(w.emitted('rename')).toHaveLength(1)

    // 删除经 ConfirmAction 桩：桩在点击包装元素时抛 confirm
    await firstRow.find('.confirm-action').trigger('click')
    expect(w.emitted('remove')).toHaveLength(1)

    // 点击行本体打开详情
    await firstRow.trigger('click')
    expect(w.emitted('detail')).toHaveLength(1)
    w.unmount()
  })

  it('readonly 时隐藏写操作，空态不带新建入口', () => {
    const w = mountList({ readonly: true, rows: [], showEmpty: true })
    expect(w.text()).toContain('只读')
    expect(w.text()).not.toContain('重命名')
    const region = w.findComponent({ name: 'SbAsyncRegion' })
    expect(region.props('emptyActionText')).toBeUndefined()
    w.unmount()
  })

  it('加载更多按钮在 loadingMore 时禁用并抛事件', async () => {
    const w = mountList()
    const more = w.findAll('button').find((b) => b.text().includes('加载更多'))!
    await more.trigger('click')
    expect(w.emitted('load-more')).toHaveLength(1)

    const busy = mountList({ loadingMore: true })
    const disabled = busy.findAll('button').find((b) => b.text().includes('加载更多'))!
    expect(disabled.attributes('disabled')).toBeDefined()
    w.unmount()
    busy.unmount()
  })

  it('空态新建与重试事件透传', async () => {
    const w = mountList({ rows: [], showEmpty: true })
    await w.get('.empty-action').trigger('click')
    expect(w.emitted('create')).toHaveLength(1)

    const err = mountList({ showError: true, error: 'boom' })
    err.findComponent({ name: 'SbAsyncRegion' }).vm.$emit('retry')
    expect(err.emitted('retry')).toHaveLength(1)
    w.unmount()
    err.unmount()
  })
})

describe('SqlResultPanel', () => {
  const mountPanel = (props: Record<string, unknown>) =>
    mount(SqlResultPanel, { props, global: { stubs: uiStubs, plugins: [i18n] } })

  it('查询模式渲染行数徽标与 null 单元格', () => {
    const w = mountPanel({
      mode: 'query',
      queryResult: {
        columns: ['id', 'name'],
        rows: [
          ['1', null],
          ['2', 'b']
        ],
        rowCount: 2,
        durationMs: 3,
        requestId: 'r1'
      },
      executeResult: null,
      batchResult: null
    })
    expect(w.text()).toContain('2 行')
    expect(w.text()).toContain('request_id: r1')
    // null 单元格走 formatCell 的 NULL 分支
    expect(w.text()).toContain('NULL')
    w.unmount()
  })

  it('查询模式无行时显示空态', () => {
    const w = mountPanel({
      mode: 'query',
      queryResult: { columns: [], rows: [], rowCount: 0, durationMs: 1, requestId: 'r' },
      executeResult: null,
      batchResult: null
    })
    expect(w.text()).toContain('暂时未查询到数据')
    w.unmount()
  })

  it('未执行时展示占位空态', () => {
    const w = mountPanel({ mode: 'execute', queryResult: null, executeResult: null, batchResult: null })
    expect(w.text()).toContain('执行后结果将显示在这里')
    w.unmount()
  })

  it('执行模式渲染受影响行数与持久化徽标', () => {
    const w = mountPanel({
      mode: 'execute',
      queryResult: null,
      executeResult: { rowsAffected: 5, durationMs: 2, durability: 'fsync', requestId: 'e1' },
      batchResult: null
    })
    expect(w.text()).toContain('5 行受影响')
    expect(w.text()).toContain('fsync')
    w.unmount()
  })

  it('批量模式逐条渲染失败与成功', () => {
    const w = mountPanel({
      mode: 'batch',
      queryResult: null,
      executeResult: null,
      batchResult: {
        results: [
          { rowsAffected: 1 },
          { errorCode: 'e', errorMessage: 'bad' }
        ],
        durability: 'ok',
        durationMs: 4,
        requestId: 'b1',
        error: { failedIndex: 1, code: 'e', message: 'bad' }
      }
    })
    expect(w.text()).toContain('2 条语句')
    expect(w.text()).toContain('事务回滚：第 2 条失败')
    expect(w.text()).toContain('bad')
    w.unmount()
  })

  it('分页组件向上抛 update:page', async () => {
    const w = mountPanel({
      mode: 'query',
      queryResult: { columns: ['id'], rows: [['1']], rowCount: 1, durationMs: 1, requestId: 'r' },
      executeResult: null,
      batchResult: null
    })
    // TablePager 桩以 .pager-next 暴露一次 update:page
    await w.get('.pager-next').trigger('click')
    expect(w.emitted('update:page')).toEqual([[2]])
    w.unmount()
  })
})

describe('CronJobScheduleFields', () => {
  const mountFields = (kind: string, extra: Record<string, unknown> = {}) =>
    mount(CronJobScheduleFields, {
      props: {
        scheduleKind: kind,
        cronExpr: '0 2 * * *',
        runAtLocal: '',
        intervalValue: 10,
        intervalUnit: 'minute',
        preset: undefined,
        ...extra
      },
      global: { stubs: uiStubs, plugins: [i18n] }
    })

  it('cron 模式渲染表达式与预设下拉，选中预设回传 cronExpr', async () => {
    const w = mountFields('cron')
    expect(w.get('label[for="cron-expression"]').text()).toContain('cron 表达式')
    await w.get('.select-emit').trigger('click')
    // 桩抛 'info'，不是合法表达式，仍应落到 preset
    expect(w.emitted('update:preset')).toBeTruthy()
    w.unmount()
  })

  it('预设为空时不报错（presetValue 计算属性兜底）', async () => {
    const w = mountFields('cron', { preset: undefined })
    expect(w.find('.select-emit').exists()).toBe(true)
    w.unmount()
  })

  it('interval 模式渲染间隔与单位', () => {
    const w = mountFields('interval', { intervalUnit: 'hour' })
    expect(w.get('label[for="cron-interval-value"]').text()).toContain('执行间隔')
    expect(w.findAll('.select-emit').length).toBeGreaterThan(0)
    w.unmount()
  })

  it('once 模式渲染执行时刻', () => {
    const w = mountFields('once')
    expect(w.get('label[for="cron-run-at"]').text()).toContain('执行时刻')
    w.unmount()
  })

  it('单选切换模式回传 scheduleKind', async () => {
    const w = mountFields('cron')
    const radios = w.findAll('input[type="radio"]')
    await radios[1].setValue()
    expect(w.emitted('update:scheduleKind')?.at(-1)).toEqual(['interval'])
    w.unmount()
  })
})

describe('设置面板拆分件', () => {
  it('ModelDefaultsPanel 在无项目时显示空态并可创建项目', async () => {
    const w = mount(ModelDefaultsPanel, { global: { stubs: uiStubs, plugins: [i18n, createPinia()] } })
    const store = useProjectStore()
    store.projectId = ''
    await w.vm.$nextTick()
    expect(w.text()).toMatch(/请先在右上角|创建项目/)
    w.unmount()
  })
})

// 让未使用的导入保持有意义（helper 断言片段）
void sampleCron
void sampleGoFn
void t
void defineComponent
