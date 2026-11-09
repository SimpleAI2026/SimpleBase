import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { ADMIN_PROJECT_ID } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, sampleCron } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import CronJobs from '@/pages/CronJobs.vue'

const extraStubs = {
  CronJobModal: {
    props: ['open', 'target'],
    emits: ['saved', 'update:open'],
    template:
      '<div v-if="open" class="cj-modal"><button type="button" class="cj-close" @click="$emit(\'update:open\', false)">x</button>{{ target ? target.name : \'new\' }}</div>'
  },
  CronJobRunsModal: {
    props: ['open', 'job', 'autoTrigger'],
    emits: ['triggered', 'update:open'],
    template:
      '<div v-if="open" class="runs"><button type="button" class="runs-close" @click="$emit(\'update:open\', false)">x</button>{{ job && job.name }}</div>'
  }
}

describe('CronJobs (定时任务)', () => {
  beforeEach(() => {
    resetApiMocks()
    api.cronjobs.list.mockResolvedValue([
      sampleCron,
      {
        ...sampleCron,
        id: 'cj-2',
        name: 'hourly',
        scheduleKind: 'interval',
        intervalSeconds: 3600,
        lastStatus: 'failed',
        targetMissing: true,
        enabled: false
      },
      {
        ...sampleCron,
        id: 'cj-3',
        name: 'daily',
        scheduleKind: 'interval',
        intervalSeconds: 86400,
        lastStatus: 'running',
        nextRunAt: undefined,
        lastRunAt: undefined
      },
      {
        ...sampleCron,
        id: 'cj-4',
        name: 'mins',
        scheduleKind: 'interval',
        intervalSeconds: 120,
        lastStatus: ''
      },
      {
        ...sampleCron,
        id: 'cj-5',
        name: 'secs',
        scheduleKind: 'interval',
        intervalSeconds: 45,
        lastStatus: 'queued'
      }
    ])
  })

  it('renders schedule text, status badges, and missing-target warning', async () => {
    const { wrapper } = await mountWithApp(CronJobs, { stubs: extraStubs })
    expect(wrapper.text()).toContain('0 2 * * *')
    expect(wrapper.text()).toContain('每 1 小时')
    expect(wrapper.text()).toContain('每 1 天')
    expect(wrapper.text()).toContain('每 2 分钟')
    expect(wrapper.text()).toContain('每 45 秒')
    expect(wrapper.text()).toContain('成功')
    expect(wrapper.text()).toContain('失败')
    expect(wrapper.text()).toContain('执行中')
    expect(wrapper.text()).toContain('未运行')
    expect(wrapper.text()).toContain('hello.Hello')
  })

  it('toggles, triggers, edits, and deletes jobs', async () => {
    const { wrapper } = await mountWithApp(CronJobs, { stubs: extraStubs })
    await wrapper.get('.switch').trigger('click')
    await flushPromises()
    expect(api.cronjobs.update).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()

    api.cronjobs.update.mockRejectedValueOnce(new Error('toggle'))
    await wrapper.get('.switch').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('toggle')

    // 「立即执行」只负责打开弹窗（autoTrigger），触发由弹窗内部完成
    await clickText(wrapper, '立即执行')
    await flushPromises()
    expect(wrapper.find('.runs').exists()).toBe(true)
    expect(api.cronjobs.trigger).not.toHaveBeenCalled()

    await clickText(wrapper, '记录')
    expect(wrapper.find('.runs').exists()).toBe(true)
    await clickText(wrapper, '编辑')
    expect(wrapper.get('.cj-modal').text()).toContain('nightly')
    await clickText(wrapper, '新建定时任务')
    expect(wrapper.get('.cj-modal').text()).toContain('new')
    await wrapper.get('.cj-close').trigger('click')

    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.cronjobs.remove).toHaveBeenCalled()
    api.cronjobs.remove.mockRejectedValueOnce(new Error('rm'))
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm')
    await wrapper.get('.pager-next').trigger('click')
  })

  it('hides writes on admin and handles empty / error / project change', async () => {
    api.cronjobs.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(CronJobs, { projectId: ADMIN_PROJECT_ID, stubs: extraStubs })
    expect(wrapper.text()).toContain('系统项目不支持定时任务')

    api.cronjobs.list.mockRejectedValueOnce(new Error('list'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('list')
  })

  it('opens create from empty-state action', async () => {
    api.cronjobs.list.mockResolvedValueOnce([])
    const { wrapper, pinia } = await mountWithApp(CronJobs, { stubs: extraStubs })
    await wrapper.get('.empty-action').trigger('click')
    expect(wrapper.find('.cj-modal').exists()).toBe(true)
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.cronjobs.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('reloads the catalog from the toolbar refresh button', async () => {
    const { wrapper } = await mountWithApp(CronJobs, { stubs: extraStubs })
    const before = api.cronjobs.list.mock.calls.length
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(api.cronjobs.list.mock.calls.length).toBeGreaterThan(before)
    expect(wrapper.text()).toContain('定时任务列表')
  })
  it('handles non-Error responses', async () => {
    const cron = await mountWithApp(CronJobs)
    const cvm = cron.wrapper.vm as Record<string, any>
    expect(cvm.scheduleText({ scheduleKind: 'interval', intervalSeconds: undefined })).toContain('每')
    api.cronjobs.update.mockRejectedValueOnce('tog')
    await cvm.toggleEnabled(sampleCron)
    api.cronjobs.trigger.mockRejectedValueOnce('trig')
    await cvm.trigger(sampleCron)
    api.cronjobs.remove.mockRejectedValueOnce('rm')
    await cvm.remove(sampleCron)
    cvm.openCreate()
    cvm.modalOpen = false
    cron.wrapper.unmount()

  })

  it('CronJobs toggle/trigger/remove and schedule text', async () => {
    const { wrapper: w } = await mountWithApp(CronJobs, { stubs: extraStubs })
    await flushPromises()
    const vm = w.vm as any
    const rec = {
      id: 'j',
      name: 'night',
      scheduleKind: 'interval',
      intervalSeconds: 120,
      enabled: true
    }
    expect(vm.scheduleText({ scheduleKind: 'cron', cronExpr: '* * * * *' })).toContain('*')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 86400 })).toContain('天')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 3600 })).toContain('小时')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 120 })).toContain('分钟')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 61 })).toContain('秒')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 61 })).toContain('秒')
    api.cronjobs.update.mockResolvedValue({ ...rec, enabled: false })
    await vm.toggleEnabled(rec)
    api.cronjobs.update.mockRejectedValueOnce(new Error('x'))
    await vm.toggleEnabled(rec)
    await vm.trigger(rec)
    api.cronjobs.trigger.mockRejectedValueOnce(new Error('x'))
    await vm.trigger(rec)
    await vm.remove(rec)
    api.cronjobs.remove.mockRejectedValueOnce(new Error('x'))
    await vm.remove(rec)
    vm.openCreate()
    vm.openEdit(rec)
    vm.openRuns(rec)
    api.cronjobs.list.mockRejectedValueOnce('bad')
    await vm.load()
    w.unmount()
  })


})
