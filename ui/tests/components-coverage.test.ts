import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import AiChat from '@/components/ai/AiChat.vue'
import AiChatComposer from '@/components/ai/AiChatComposer.vue'
import MessageScroller from '@/components/chat/MessageScroller.vue'
import CollectionPanel from '@/components/databases/CollectionPanel.vue'
import ConnectionPanel from '@/components/settings/ConnectionPanel.vue'
import CronJobRunsModal from '@/components/modal/CronJobRunsModal.vue'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'

const { api } = vi.hoisted(() => ({
  api: {
    db: { collections: vi.fn() },
    cronjobs: { runs: vi.fn(), trigger: vi.fn() },
    llm: { chat: vi.fn(), stream: vi.fn() }
  }
}))

vi.mock('@/services/api', () => ({ api, isMock: false }))

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
  SbModal: { props: ['open'], template: '<div v-if="open !== false"><slot /></div>' },
  Badge: { template: '<span><slot /></span>' },
  Input: { template: '<input />' }
}

describe('component coverage', () => {
  it('ConnectionPanel and CronJobRunsModal helpers', async () => {
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
    const runs = mount(CronJobRunsModal, {
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
