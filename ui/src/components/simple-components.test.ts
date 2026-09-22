import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import PageContainer from './PageContainer.vue'
import SbCodeBlock from './SbCodeBlock.vue'
import TablePager from './TablePager.vue'
import ConfirmAction from './ConfirmAction.vue'
import ProjectScope from './ProjectScope.vue'
import { useProjectStore } from '../stores/project'

describe('simple presentational components', () => {
  it('renders PageContainer subtitle and slot', () => {
    const w = mount(PageContainer, { props: { subtitle: 'hello' }, slots: { default: '<p>body</p>' } })
    expect(w.text()).toContain('hello')
    expect(w.text()).toContain('body')
  })

  it('formats JSON in SbCodeBlock and falls back to slot', () => {
    const json = mount(SbCodeBlock, { props: { value: { a: 1 }, maxHeight: '80px' } })
    expect(json.text()).toContain('"a"')
    const slotted = mount(SbCodeBlock, { slots: { default: 'raw' } })
    expect(slotted.text()).toContain('raw')
  })

  it('emits pager updates only when there are multiple pages', async () => {
    const single = mount(TablePager, { props: { page: 1, pageSize: 10, total: 3, pageCount: 1 } })
    expect(single.text()).toContain('共 3 条')
    expect(single.findAll('button')).toHaveLength(0)

    expect(single.classes().join(' ')).toContain('pt-3')

    const footer = mount(TablePager, {
      props: { page: 1, pageSize: 10, total: 3, pageCount: 1, variant: 'footer' }
    })
    expect(footer.classes().join(' ')).toContain('border-t')
    expect(footer.classes().join(' ')).not.toContain('pt-3')

    const multi = mount(TablePager, { props: { page: 2, pageSize: 10, total: 30, pageCount: 3 } })
    await multi.findAll('button')[0].trigger('click')
    expect(multi.emitted('update:page')?.[0]).toEqual([1])
    await multi.findAll('button')[1].trigger('click')
    expect(multi.emitted('update:page')?.[1]).toEqual([3])
  })

  it('ConfirmAction renders the slot when disabled', () => {
    const w = mount(ConfirmAction, {
      props: { title: 't', disabled: true },
      slots: { default: '<button>go</button>' }
    })
    expect(w.text()).toContain('go')
  })

  it('ProjectScope shows empty state without a project id', async () => {
    setActivePinia(createPinia())
    const store = useProjectStore()
    store.projectId = ''
    const w = mount(ProjectScope, {
      global: {
        stubs: {
          Card: { template: '<div><slot /></div>' },
          CardContent: { template: '<div><slot /></div>' },
          SbEmptyState: {
            template: '<button @click="$emit(\'action\')">empty</button>',
            emits: ['action']
          }
        }
      }
    })
    expect(w.text()).toContain('empty')
    await w.get('button').trigger('click')
    expect(store.createModalOpen).toBe(true)
  })
})

describe('router-backed chrome', () => {
  it('NavMenu lists console routes', async () => {
    const { default: NavMenu } = await import('./NavMenu.vue')
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'dashboard', component: { template: '<div />' }, meta: { title: '监控大盘' } },
        { path: '/docs', name: 'docs', component: { template: '<div />' }, meta: { title: '文档', hidden: true } }
      ]
    })
    await router.push('/')
    await router.isReady()
    const w = mount(NavMenu, {
      global: {
        plugins: [router],
        stubs: {
          SidebarMenu: { template: '<div><slot /></div>' },
          SidebarMenuItem: { template: '<div><slot /></div>' },
          SidebarMenuButton: { template: '<div><slot /></div>' }
        }
      }
    })
    expect(w.text()).toContain('监控大盘')
    expect(w.text()).not.toContain('设置')
    const names = (w.vm as { menuItems: { name: string }[] }).menuItems.map((i) => i.name)
    expect(names).toEqual(['dashboard', 'databases', 's3', 'gofunctions', 'cron-jobs', 'agents', 'logs'])
  })
})
