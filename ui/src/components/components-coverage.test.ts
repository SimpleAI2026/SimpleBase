import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import AiChat from './ai/AiChat.vue'
import AiChatComposer from './ai/AiChatComposer.vue'
import MessageScroller from './chat/MessageScroller.vue'
import CollectionPanel from './databases/CollectionPanel.vue'
import ConnectionPanel from './settings/ConnectionPanel.vue'
import CronJobRunsDrawer from './CronJobRunsDrawer.vue'
import { useAuthStore } from '../stores/auth'
import { useProjectStore } from '../stores/project'

const { api } = vi.hoisted(() => ({
  api: {
    db: { collections: vi.fn() },
    cronjobs: { runs: vi.fn(), trigger: vi.fn() },
    llm: { chat: vi.fn(), stream: vi.fn() }
  }
}))

vi.mock('../services/api', () => ({ api, isMock: false }))

const stubs = {
  Select: { template: '<div><slot /></div>' },
  SelectTrigger: { template: '<div />' },
  SelectValue: { template: '<div />' },
  SelectContent: { template: '<div><slot /></div>' },
  SelectGroup: { template: '<div><slot /></div>' },
  SelectItem: { template: '<div><slot /></div>' },
  Switch: { template: '<button />' },
  Button: { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  Card: { template: '<div><slot /></div>' },
  CardHeader: { template: '<div><slot /></div>' },
  CardTitle: { template: '<div><slot /></div>' },
  CardContent: { template: '<div><slot /></div>' },
  SbEmptyState: { template: '<div>empty</div>' },
  MessageScroller: { template: '<div><slot /></div>' },
  Message: { template: '<div><slot /></div>' },
  MessageAvatar: { template: '<div><slot /></div>' },
  MessageContent: { template: '<div><slot /></div>' },
  Bubble: { template: '<div><slot /></div>' },
  AiChatComposer: { template: '<div />' },
  Popover: { template: '<div><slot /></div>' },
  PopoverTrigger: { template: '<div><slot /></div>' },
  PopoverContent: { template: '<div><slot /></div>' },
  Command: { template: '<div><slot /></div>' },
  CommandList: { template: '<div><slot /></div>' },
  CommandEmpty: { template: '<div />' },
  CommandGroup: { template: '<div><slot /></div>' },
  CommandItem: { template: '<div><slot /></div>' },
  Table: { template: '<table><slot /></table>' },
  TableHeader: { template: '<thead><slot /></thead>' },
  TableBody: { template: '<tbody><slot /></tbody>' },
  TableRow: { template: '<tr><slot /></tr>' },
  TableHead: { template: '<th><slot /></th>' },
  TableCell: { template: '<td><slot /></td>' },
  TableEmpty: { template: '<tr><slot /></tr>' },
  Skeleton: { template: '<div />' },
  Sheet: { template: '<div><slot /></div>' },
  SheetContent: { template: '<div><slot /></div>' },
  SheetHeader: { template: '<div><slot /></div>' },
  SheetTitle: { template: '<div><slot /></div>' },
  SheetDescription: { template: '<div><slot /></div>' },
  Badge: { template: '<span><slot /></span>' },
  Input: { template: '<input />' }
}

describe('component coverage', () => {
  it('AiChat customSend, default send, stop and clear', async () => {
    api.llm.chat.mockResolvedValue({ content: 'ok', model: 'm', provider: 'p' })
    const customSend = vi.fn().mockResolvedValue(undefined)
    const w = mount(AiChat, {
      props: {
        projectId: 'p',
        model: 'm',
        streaming: false,
        customSend,
        messages: [{ role: 'user', content: 'hi', toolCalls: [{ name: 't', arguments: '{}', content: 'c' }] }],
        sending: false,
        modelOptions: [{ label: 'm', value: 'm' }]
      },
      global: { stubs }
    })
    const vm = w.vm as any
    vm.draft = '  '
    await vm.onSend([])
    vm.draft = 'hello'
    await vm.onSend([{ agent_id: 'a' }])
    expect(customSend).toHaveBeenCalled()
    vm.stop()
    vm.clear()
    w.unmount()

    const w2 = mount(AiChat, {
      props: { projectId: 'p', model: 'm', streaming: false, showToolbar: true },
      global: { stubs }
    })
    const vm2 = w2.vm as any
    vm2.draft = 'ping'
    await vm2.onSend([])
    vm2.stop()
    vm2.clear()
    w2.unmount()
  })

  it('AiChatComposer mention parsing and send', async () => {
    const w = mount(AiChatComposer, {
      props: {
        modelValue: 'hi @Bot',
        mentionAgents: [{ id: 'a1', name: 'Bot', module: 'db' }]
      },
      global: { stubs }
    })
    const vm = w.vm as any
    expect(vm.mentionsForSend('hi @Bot')).toEqual([{ agent_id: 'a1' }])
    vm.parseMentionQuery('hello')
    vm.parseMentionQuery('hi @Bo')
    vm.parseMentionQuery('hi @Bo more')
    vm.pickMention({ id: 'a1', name: 'Bot' })
    vm.pickMention({ id: 'a1', name: 'Bot' })
    vm.onInput({ target: { value: 'x @Z' } })
    vm.onKeydown({ key: 'Escape', preventDefault: vi.fn() })
    vm.mentionOpen = true
    vm.onKeydown({ key: 'Escape', preventDefault: vi.fn() })
    vm.onKeydown({ key: 'Enter', shiftKey: true, preventDefault: vi.fn() })
    const prevent = vi.fn()
    vm.onKeydown({ key: 'Enter', shiftKey: false, preventDefault: prevent })
    await flushPromises()
    w.unmount()
  })

  it('MessageScroller stick/scroll helpers', async () => {
    const w = mount(MessageScroller, { props: { followKey: '1' }, global: { stubs } })
    const vm = w.vm as any
    vm.onScroll()
    vm.viewport = { scrollHeight: 400, scrollTop: 10, clientHeight: 100 }
    vm.onScroll()
    expect(vm.stick).toBe(false)
    vm.viewport = { scrollHeight: 400, scrollTop: 350, clientHeight: 100 }
    vm.onScroll()
    expect(vm.stick).toBe(true)
    vm.scrollToEnd()
    await w.setProps({ followKey: '2' })
    await flushPromises()
    w.unmount()
  })

  it('CollectionPanel loads collections and errors', async () => {
    api.db.collections.mockResolvedValue(['users'])
    const w = mount(CollectionPanel, {
      props: { projectId: 'p', database: { id: 'd', name: 'n', status: 'ready', createdAt: 't', updatedAt: 't' } },
      global: { stubs }
    })
    await flushPromises()
    expect(api.db.collections).toHaveBeenCalled()
    api.db.collections.mockRejectedValueOnce(new Error('x'))
    await w.setProps({ reloadToken: 2 })
    await flushPromises()
    w.unmount()
  })

  it('ConnectionPanel and CronJobRunsDrawer helpers', async () => {
    setActivePinia(createPinia())
    useProjectStore().setProject('p', 'Proj')
    const auth = useAuthStore()
    auth.settingsOpen = true
    const panel = mount(ConnectionPanel, { global: { plugins: [createPinia()], stubs } })
    await flushPromises()
    panel.unmount()

    api.cronjobs.runs.mockResolvedValue([
      { id: '1', jobId: 'j', trigger: 'manual', status: 'completed', error: '', durationMs: 1, responseJson: '{"a":1}', createdAt: 't' }
    ])
    const runs = mount(CronJobRunsDrawer, {
      props: {
        open: true,
        job: {
          id: 'j',
          name: 'night',
          description: '',
          scheduleKind: 'cron',
          cronExpr: '* * * * *',
          funcFile: 'hello',
          funcExport: 'Hello',
          inputJson: '{}',
          enabled: true,
          lastStatus: 'completed',
          lastError: '',
          runCount: 1,
          targetMissing: false,
          createdAt: 't',
          updatedAt: 't'
        }
      },
      global: { stubs }
    })
    await flushPromises()
    const rvm = runs.vm as any
    await rvm.load?.()
    await rvm.trigger?.()
    api.cronjobs.runs.mockRejectedValueOnce(new Error('x'))
    await rvm.load?.()
    api.cronjobs.trigger.mockRejectedValueOnce(new Error('x'))
    await rvm.trigger?.()
    runs.unmount()
  })
})
