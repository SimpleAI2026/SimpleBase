import { flushPromises, mount, type ComponentMountingOptions, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { createMemoryHistory, createRouter, type RouteRecordRaw, type Router } from 'vue-router'
import type { Component } from 'vue'
import { DEFAULT_PROJECT_ID, useProjectStore } from '../stores/project'

export const uiStubs = {
  Button: { inheritAttrs: true, template: '<button v-bind="$attrs"><slot /></button>' },
  Card: { template: '<div class="card"><slot /></div>' },
  CardHeader: { template: '<div class="card-header"><slot /></div>' },
  CardTitle: { template: '<div class="card-title"><slot /></div>' },
  CardDescription: { template: '<div class="card-desc"><slot /></div>' },
  CardContent: { template: '<div class="card-content"><slot /></div>' },
  CardAction: { template: '<div class="card-action"><slot /></div>' },
  CardFooter: { template: '<div class="card-footer"><slot /></div>' },
  Badge: { template: '<span class="badge"><slot /></span>' },
  Skeleton: { template: '<div class="skeleton" />' },
  Spinner: { template: '<span class="spinner" />' },
  Progress: { template: '<div class="progress" />' },
  Separator: { template: '<hr />' },
  Alert: { template: '<div class="alert"><slot /></div>' },
  AlertTitle: { template: '<div><slot /></div>' },
  AlertDescription: { template: '<div><slot /></div>' },
  Table: { template: '<table><slot /></table>' },
  TableHeader: { template: '<thead><slot /></thead>' },
  TableBody: { template: '<tbody><slot /></tbody>' },
  TableRow: { template: '<tr><slot /></tr>' },
  TableHead: { template: '<th><slot /></th>' },
  TableCell: { template: '<td><slot /></td>' },
  TableEmpty: { template: '<tr class="table-empty"><td><slot /></td></tr>' },
  Tooltip: { template: '<div><slot /></div>' },
  TooltipTrigger: { template: '<div><slot /></div>' },
  TooltipContent: { template: '<div><slot /></div>' },
  TooltipProvider: { template: '<div><slot /></div>' },
  Dialog: { template: '<div class="dialog"><slot /></div>' },
  DialogContent: {
    inheritAttrs: false,
    template: '<div class="dialog-content" :class="$attrs.class" :style="$attrs.style"><slot /></div>'
  },
  DialogHeader: { template: '<div><slot /></div>' },
  DialogTitle: { template: '<div><slot /></div>' },
  DialogDescription: { template: '<div><slot /></div>' },
  DialogFooter: { template: '<div class="dialog-footer"><slot /></div>' },
  Sheet: {
    props: ['open'],
    emits: ['update:open'],
    template:
      '<div v-if="open !== false" class="sheet"><button type="button" class="sheet-close" @click="$emit(\'update:open\', false)">x</button><slot /></div>'
  },
  SheetContent: {
    emits: ['close'],
    template:
      '<div class="sheet-content"><button type="button" class="sheet-content-close" @click="$emit(\'close\')">x</button><slot /></div>'
  },
  SheetHeader: { template: '<div><slot /></div>' },
  SheetTitle: { template: '<div><slot /></div>' },
  SheetDescription: { template: '<div><slot /></div>' },
  Select: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: `<div class="select">
      <button type="button" class="select-emit" @click="$emit('update:modelValue', 'info')">sel</button>
      <button type="button" class="select-hour" @click="$emit('update:modelValue', 'hour')">hour</button>
      <button type="button" class="select-day" @click="$emit('update:modelValue', 'day')">day</button>
      <button type="button" class="select-custom" @click="$emit('update:modelValue', 'custom')">custom</button>
      <button type="button" class="select-empty" @click="$emit('update:modelValue', '')">empty</button>
      <slot />
    </div>`
  },
  SelectTrigger: { template: '<div><slot /></div>' },
  SelectValue: { template: '<span />' },
  SelectContent: { template: '<div><slot /></div>' },
  SelectGroup: { template: '<div><slot /></div>' },
  SelectItem: { props: ['value'], template: '<option :value="value"><slot /></option>' },
  Combobox: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<div class="combo"><button type="button" class="combo-emit" @click="$emit(\'update:modelValue\', \'gpt-4o-mini\')">combo</button><slot /></div>'
  },
  ComboboxAnchor: { template: '<div><slot /></div>' },
  ComboboxInput: { template: '<input />' },
  ComboboxList: { template: '<div><slot /></div>' },
  ComboboxViewport: { template: '<div><slot /></div>' },
  ComboboxEmpty: { template: '<div><slot /></div>' },
  ComboboxGroup: { template: '<div><slot /></div>' },
  ComboboxItem: { template: '<div><slot /></div>' },
  ToggleGroup: {
    props: ['modelValue', 'type'],
    emits: ['update:modelValue'],
    template:
      '<div class="tg"><button type="button" class="tg-light" @click="$emit(\'update:modelValue\', \'light\')">浅色</button><button type="button" class="tg-dark" @click="$emit(\'update:modelValue\', \'dark\')">深色</button><button type="button" class="tg-system" @click="$emit(\'update:modelValue\', \'system\')">跟随系统</button><button type="button" class="tg-query" @click="$emit(\'update:modelValue\', \'query\')">查询</button><button type="button" class="tg-execute" @click="$emit(\'update:modelValue\', \'execute\')">执行</button><button type="button" class="tg-batch" @click="$emit(\'update:modelValue\', \'batch\')">批量</button><button type="button" class="tg-bar" @click="$emit(\'update:modelValue\', \'bar\')">柱状图</button><button type="button" class="tg-line" @click="$emit(\'update:modelValue\', \'line\')">折线图</button><slot /></div>'
  },
  ToggleGroupItem: { props: ['value'], template: '<span class="toggle-item"><slot /></span>' },
  Slider: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<button type="button" class="slider-emit" @click="$emit(\'update:modelValue\', [0.5])">slider</button>'
  },
  Switch: {
    props: ['checked'],
    emits: ['update:checked'],
    template: '<button type="button" class="switch" @click="$emit(\'update:checked\', !checked)">sw</button>'
  },
  Input: {
    props: ['modelValue', 'id', 'type', 'placeholder', 'disabled'],
    emits: ['update:modelValue'],
    inheritAttrs: false,
    template:
      '<input :id="id" :type="type || \'text\'" :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" />'
  },
  Textarea: {
    props: ['modelValue', 'id', 'rows'],
    emits: ['update:modelValue'],
    template:
      '<textarea :id="id" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)"></textarea>'
  },
  InputGroup: { template: '<div><slot /></div>' },
  InputGroupAddon: { template: '<div><slot /></div>' },
  InputGroupInput: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<input class="ig-input" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keydown="$emit(\'keydown\', $event)" />'
  },
  Field: { template: '<div class="field"><slot /></div>' },
  FieldGroup: { template: '<div><slot /></div>' },
  FieldLabel: { template: '<label><slot /></label>' },
  FieldTitle: { template: '<div><slot /></div>' },
  FieldDescription: { template: '<div><slot /></div>' },
  FieldError: { template: '<div class="field-error"><slot /></div>' },
  FieldSet: { template: '<div><slot /></div>' },
  FieldLegend: { template: '<div><slot /></div>' },
  FieldContent: { template: '<div><slot /></div>' },
  Label: { template: '<label><slot /></label>' },
  Avatar: { template: '<div><slot /></div>' },
  AvatarFallback: { template: '<div><slot /></div>' },
  Breadcrumb: { template: '<nav><slot /></nav>' },
  BreadcrumbList: { template: '<div><slot /></div>' },
  BreadcrumbItem: { template: '<div><slot /></div>' },
  BreadcrumbPage: { template: '<span><slot /></span>' },
  BreadcrumbLink: { template: '<span><slot /></span>' },
  BreadcrumbSeparator: { template: '<span>/</span>' },
  Tabs: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<div class="tabs"><button type="button" class="tab-emit" @click="$emit(\'update:modelValue\', \'other\')">tab</button><slot /></div>'
  },
  TabsList: { template: '<div class="tabs-list"><slot /></div>' },
  TabsTrigger: {
    props: ['value'],
    template: '<button type="button" class="tab-trigger" :data-tab="value"><slot /></button>'
  },
  TabsContent: {
    props: ['value'],
    template: '<div class="tabs-content" :data-tab="value"><slot /></div>'
  },
  SidebarProvider: { template: '<div class="sidebar-provider"><slot /></div>' },
  Sidebar: { template: '<aside><slot /></aside>' },
  SidebarHeader: { template: '<div><slot /></div>' },
  SidebarContent: { template: '<div><slot /></div>' },
  SidebarGroup: { template: '<div><slot /></div>' },
  SidebarGroupContent: { template: '<div><slot /></div>' },
  SidebarRail: { template: '<div />' },
  SidebarInset: { template: '<div class="inset"><slot /></div>' },
  SidebarTrigger: { template: '<button type="button">sb</button>' },
  SidebarMenu: { template: '<div><slot /></div>' },
  SidebarMenuItem: { template: '<div><slot /></div>' },
  SidebarMenuButton: { template: '<div><slot /></div>' },
  ScrollArea: { template: '<div><slot /></div>' },
  Popover: {
    props: ['open'],
    emits: ['update:open'],
    template:
      '<div class="popover"><button type="button" class="pop-open" @click="$emit(\'update:open\', true)">open</button><button type="button" class="pop-close" @click="$emit(\'update:open\', false)">close</button><slot /></div>'
  },
  PopoverTrigger: { template: '<div><slot /></div>' },
  PopoverContent: { template: '<div><slot /></div>' },
  Command: { template: '<div><slot /></div>' },
  CommandInput: { template: '<input class="cmd-input" />' },
  CommandList: { template: '<div><slot /></div>' },
  CommandEmpty: { template: '<div><slot /></div>' },
  CommandGroup: { template: '<div><slot /></div>' },
  CommandItem: {
    props: ['value'],
    emits: ['select'],
    template: '<button type="button" class="cmd-item" @click="$emit(\'select\')"><slot /></button>'
  },
  AlertDialog: { template: '<div><slot /></div>' },
  AlertDialogTrigger: { template: '<div><slot /></div>' },
  AlertDialogContent: { template: '<div><slot /></div>' },
  AlertDialogHeader: { template: '<div><slot /></div>' },
  AlertDialogTitle: { template: '<div><slot /></div>' },
  AlertDialogDescription: { template: '<div><slot /></div>' },
  AlertDialogFooter: { template: '<div><slot /></div>' },
  AlertDialogCancel: { template: '<button type="button">取消</button>' },
  AlertDialogAction: {
    template: '<button type="button" class="ad-ok" @click="$emit(\'click\')">确定</button>'
  },
  Empty: { template: '<div class="empty-root"><slot /></div>' },
  EmptyHeader: { template: '<div><slot /></div>' },
  EmptyMedia: { template: '<div><slot /></div>' },
  EmptyTitle: { template: '<div><slot /></div>' },
  EmptyDescription: { template: '<div><slot /></div>' },
  EmptyContent: { template: '<div><slot /></div>' },
  SbModal: {
    props: {
      open: Boolean,
      title: String,
      width: [Number, String],
      maxWidth: [Number, String],
      minWidth: [Number, String],
      hideFooter: Boolean,
      confirmLoading: Boolean,
      okButtonProps: Object,
      okText: String,
      cancelText: String,
      description: String
    },
    emits: ['ok', 'update:open', 'cancel'],
    template: `
      <div v-if="open" class="sb-modal" :data-title="title" :data-width="width" :data-max-width="maxWidth" :data-min-width="minWidth" :data-hide-footer="hideFooter ? 'true' : 'false'">
        <p v-if="description" class="sb-modal-desc">{{ description }}</p>
        <slot />
        <slot v-if="!hideFooter" name="footer">
          <button type="button" class="sb-ok" @click="$emit('ok')">{{ okText || '确定' }}</button>
          <button type="button" class="sb-cancel" @click="$emit('cancel'); $emit('update:open', false)">{{ cancelText || '取消' }}</button>
        </slot>
      </div>
    `
  },
  ConfirmAction: {
    props: ['disabled', 'title'],
    emits: ['confirm'],
    template: '<div class="confirm-action" @click="!disabled && $emit(\'confirm\')"><slot /></div>'
  },
  SbEmptyState: {
    props: ['title', 'description', 'actionText'],
    emits: ['action'],
    template:
      '<div class="empty-state">{{ title }} {{ description }}<button v-if="actionText" type="button" class="empty-action" @click="$emit(\'action\')">{{ actionText }}</button></div>'
  },
  ProjectScope: { template: '<div class="project-scope"><slot /></div>' },
  PageContainer: { props: ['subtitle'], template: '<div class="page"><p>{{ subtitle }}</p><slot /></div>' },
  TablePager: {
    props: ['page', 'pageSize', 'total', 'pageCount'],
    emits: ['update:page'],
    template: '<button type="button" class="pager-next" @click="$emit(\'update:page\', (page || 1) + 1)">next</button>'
  }
}

export const defaultRoutes: RouteRecordRaw[] = [
  { path: '/', name: 'dashboard', component: { template: '<div>dash</div>' }, meta: { title: '监控大盘' } },
  { path: '/databases', name: 'databases', component: { template: '<div>db</div>' }, meta: { title: '数据库管理' } },
  { path: '/s3', name: 's3', component: { template: '<div>s3</div>' }, meta: { title: 'S3 对象存储' } },
  { path: '/gofunctions', name: 'gofunctions', component: { template: '<div>go</div>' }, meta: { title: '云函数' } },
  { path: '/cron-jobs', name: 'cron-jobs', component: { template: '<div>cron</div>' }, meta: { title: '定时任务' } },
  { path: '/agents', name: 'agents', component: { template: '<div>ag</div>' }, meta: { title: '云 Agent' } },
  { path: '/settings', name: 'settings', component: { template: '<div>set</div>' }, meta: { title: '设置' } },
  { path: '/logs', name: 'logs', component: { template: '<div>log</div>' }, meta: { title: '日志管理' } },
  { path: '/docs/:module?/:slug?', name: 'docs-page', component: { template: '<div>docs</div>' }, meta: { title: '使用文档' } },
  { path: '/docs', name: 'docs', component: { template: '<div>docs</div>' }, meta: { title: '使用文档', hidden: true } },
  { path: '/unnamed', component: { template: '<div />' } }
]

export async function mountWithApp(
  component: Component,
  options: ComponentMountingOptions<Component> & {
    path?: string
    projectId?: string
    projectName?: string
    routes?: RouteRecordRaw[]
    stubs?: Record<string, unknown>
  } = {}
): Promise<{ wrapper: VueWrapper; router: Router; pinia: Pinia }> {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: options.routes || defaultRoutes
  })
  await router.push(options.path || '/')
  await router.isReady()
  const store = useProjectStore()
  store.setProject(options.projectId || DEFAULT_PROJECT_ID, options.projectName || '测试项目')
  const { path: _p, projectId: _id, projectName: _n, routes: _r, stubs, ...mountOpts } = options
  const wrapper = mount(component, {
    ...mountOpts,
    global: {
      ...(mountOpts.global || {}),
      plugins: [pinia, router, ...((mountOpts.global as { plugins?: unknown[] } | undefined)?.plugins || [])],
      stubs: { ...uiStubs, ...(stubs || {}), ...((mountOpts.global as { stubs?: object } | undefined)?.stubs || {}) }
    }
  })
  await flushPromises()
  return { wrapper, router, pinia }
}

export function clickText(wrapper: VueWrapper, text: string) {
  const nodes = wrapper.findAll('button, a, [role="button"]')
  const el = nodes.find((n) => n.text().includes(text))
  if (!el) {
    throw new Error(`No clickable "${text}". text=${wrapper.text().slice(0, 400)}`)
  }
  return el.trigger('click')
}

export const readyDb = {
  id: 'db-1',
  name: 'demo',
  status: 'ready' as const,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
  documentCount: 42
}

export const closedDb = {
  id: 'db-2',
  name: 'closed-db',
  status: 'closed' as const,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z'
}

export const sampleAgent = {
  id: 'ag-1',
  name: 'Database',
  module: 'database',
  description: 'db helper',
  system_prompt: 'sys',
  tool_ids: ['list_databases'],
  team_enabled: false,
  created_at: 't',
  updated_at: 't'
}

export const sampleGoFn = {
  id: 'gf-1',
  name: 'hello',
  file: 'hello.go',
  description: '',
  activeVersion: 1,
  latestVersion: 1,
  published: true,
  exports: ['Hello', 'Ping'],
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-02T00:00:00Z'
}

export const sampleCron = {
  id: 'cj-1',
  name: 'nightly',
  description: 'night job',
  scheduleKind: 'cron' as const,
  cronExpr: '0 2 * * *',
  funcFile: 'hello',
  funcExport: 'Hello',
  inputJson: '{}',
  enabled: true,
  lastStatus: 'completed',
  lastError: '',
  runCount: 3,
  targetMissing: false,
  createdAt: 't',
  updatedAt: 't',
  lastRunAt: '2024-01-01T00:00:00Z',
  nextRunAt: '2024-01-02T00:00:00Z'
}
