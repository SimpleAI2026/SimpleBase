import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { ADMIN_PROJECT_ID } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp, sampleCron } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import CronJobs from './CronJobs.vue'

const extraStubs = {
  CronJobModal: {
    props: ['open', 'target'],
    emits: ['saved', 'update:open'],
    template: '<div v-if="open" class="cj-modal">{{ target ? target.name : \'new\' }}</div>'
  },
  CronJobRunsDrawer: {
    props: ['open', 'job'],
    emits: ['triggered', 'update:open'],
    template: '<div v-if="open" class="runs">{{ job && job.name }}</div>'
  }
}

describe('CronJobs', () => {
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

    await clickText(wrapper, '立即执行')
    await flushPromises()
    expect(api.cronjobs.trigger).toHaveBeenCalled()
    expect(wrapper.find('.runs').exists()).toBe(true)

    api.cronjobs.trigger.mockRejectedValueOnce(new Error('trig'))
    await wrapper.findAll('button').filter((b) => b.text().includes('立即执行'))[1].trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('trig')

    await clickText(wrapper, '记录')
    expect(wrapper.find('.runs').exists()).toBe(true)
    await clickText(wrapper, '编辑')
    expect(wrapper.get('.cj-modal').text()).toContain('nightly')
    await clickText(wrapper, '新建定时任务')
    expect(wrapper.get('.cj-modal').text()).toContain('new')

    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.cronjobs.remove).toHaveBeenCalled()
    api.cronjobs.remove.mockRejectedValueOnce(new Error('rm'))
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm')
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
    const { useProjectStore } = await import('../stores/project')
    useProjectStore(pinia).setProject('00000000-0000-0000-0000-000000000003')
    await flushPromises()
    expect(api.cronjobs.list.mock.calls.length).toBeGreaterThan(1)
  })
})
