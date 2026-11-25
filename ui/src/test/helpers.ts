import { DOMWrapper, enableAutoUnmount, flushPromises, mount, type ComponentMountingOptions, type VueWrapper } from '@vue/test-utils'
import { afterEach } from 'vitest'
import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { createMemoryHistory, createRouter, type RouteRecordRaw, type Router } from 'vue-router'
import type { Component, Plugin } from 'vue'
import { useProjectStore } from '@/stores/project'

export const uiStubs = {
  Button: { inheritAttrs: true, template: '<button v-bind="$attrs"><slot /></button>' },
  Card: { template: '<div class="card"><slot /></div>' },
  CardHeader: { template: '<div class="card-header"><slot /></div>' },
  CardTitle: { template: '<div class="card-title"><slot /></div>' },
  CardDescription: { template: '<div class="card-desc"><slot /></div>' },
  CardContent: { template: '<div class="card-content"><slot /></div>' },
  CardAction: { template: '<div class="card-action"><slot /></div>' },
  // 透传属性（aria-label / type 等）：真实 Badge 会把它们落到宿主元素上
  Badge: { props: ['as'], inheritAttrs: false, template: '<component :is="as || \'span\'" class="badge" v-bind="$attrs"><slot /></component>' },
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
  Input: {
    props: ['modelValue', 'id', 'type', 'placeholder', 'disabled'],
    emits: ['update:modelValue'],
    inheritAttrs: false,
    template:
      '<input v-bind="$attrs" :id="id" :type="type || \'text\'" :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" />'
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
  FieldDescription: { template: '<div><slot /></div>' },
  FieldError: { template: '<div class="field-error"><slot /></div>' },
  FieldSet: { template: '<div><slot /></div>' },
  FieldLegend: { template: '<div><slot /></div>' },
  FieldContent: { template: '<div><slot /></div>' },
  Label: { template: '<label><slot /></label>' },
  Breadcrumb: { template: '<nav><slot /></nav>' },
  BreadcrumbList: { template: '<div><slot /></div>' },
  BreadcrumbItem: { template: '<div><slot /></div>' },
  BreadcrumbPage: { template: '<span><slot /></span>' },
  BreadcrumbLink: { template: '<span><slot /></span>' },
  BreadcrumbSeparator: { template: '<span>/</span>' },
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
  Empty: { template: '<div class="empty-root"><slot /></div>' },
  EmptyHeader: { template: '<div><slot /></div>' },
  EmptyMedia: { template: '<div><slot /></div>' },
  EmptyTitle: { template: '<div><slot /></div>' },
  EmptyDescription: { template: '<div><slot /></div>' },
  EmptyContent: { template: '<div><slot /></div>' },
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
  { path: '/databases', name: 'databases', component: { template: '<div>db</div>' }, meta: { title: '数据管理' } },
  { path: '/s3', name: 's3', component: { template: '<div>s3</div>' }, meta: { title: '对象存储' } },
  { path: '/gofunctions', name: 'gofunctions', component: { template: '<div>go</div>' }, meta: { title: '云函数' } },
  { path: '/cron-jobs', name: 'cron-jobs', component: { template: '<div>cron</div>' }, meta: { title: '定时任务' } },
  { path: '/agents', name: 'agents', component: { template: '<div>ag</div>' }, meta: { title: '云助手' } },
  { path: '/models', name: 'models', component: { template: '<div>models</div>' }, meta: { title: '模型' } },
  { path: '/traces', name: 'traces', component: { template: '<div>traces</div>' }, meta: { title: 'Trace' } },
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
  store.setProject(options.projectId || 'testprj1', options.projectName || '测试项目')
  const { path: _p, projectId: _id, projectName: _n, routes: _r, stubs, ...mountOpts } = options
  // 用了真实弹层（stubs 里把 SbModal / Dialog / AlertDialog / Popover 等置为 false）时，内容 teleport 到 body，
  // 必须挂到文档上才能被 document 查询；调用方没显式给 attachTo 就默认挂上。
  const merged: Record<string, unknown> = { ...uiStubs, ...(stubs || {}), ...((mountOpts.global as { stubs?: Record<string, unknown> } | undefined)?.stubs || {}) }
  const realOverlay = ['SbModal', 'Dialog', 'AlertDialog', 'ConfirmAction', 'Popover', 'Select'].some((k) => merged[k] === false)
  const wrapper = mount(component, {
    ...(realOverlay && !('attachTo' in mountOpts) ? { attachTo: document.body } : {}),
    ...mountOpts,
    global: {
      ...(mountOpts.global || {}),
      plugins: [pinia, router, ...(((mountOpts.global as { plugins?: Plugin[] } | undefined)?.plugins || []) as Plugin[])],
      stubs: { ...uiStubs, ...(stubs || {}), ...((mountOpts.global as { stubs?: object } | undefined)?.stubs || {}) }
    }
  })
  await flushPromises()
  return { wrapper, router, pinia }
}

/**
 * 真实 reka 弹层（Popover / Command / Select）依赖 jsdom 没有的浏览器 API，且内容 teleport 到 body。
 *
 * 在测试文件**顶层**（describe 之外或 describe 内，而不是 beforeAll 里）调用一次：
 *   - 补齐 jsdom 缺的 scrollIntoView / pointer capture / ResizeObserver（幂等）；
 *   - 每条用例结束后自动卸载所有包装器（含 attachTo 的），弹层 Portal 内容随之移除，不会污染下一条用例。
 */
export function installJsdomPolyfills(): void {
  const proto = Element.prototype as unknown as Record<string, unknown>
  if (!proto.scrollIntoView) proto.scrollIntoView = () => undefined
  if (!proto.hasPointerCapture) proto.hasPointerCapture = () => false
  if (!proto.releasePointerCapture) proto.releasePointerCapture = () => undefined
  if (!globalThis.ResizeObserver) {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver
  }
  // 卸载 attachTo 的包装器后，reka 的 Portal 内容随组件一起被移除，无需手动清 body。
  enableAutoUnmount(afterEach)
}

/** 页面（含 teleport 到 body 的弹层）里当前渲染的 reka 选项元素：Select 的 `role=option`。 */
export function selectOptions(): HTMLElement[] {
  return Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]'))
}

/**
 * 打开真实 reka Select（不选中），返回当前列出的选项元素，调用方断言完选项后用 {@link closeSelect} 关闭。
 * 关闭状态下 reka 不渲染选项，所以「选项里有什么」必须先打开再读。
 */
export async function openSelect(trigger: { trigger: (e: string, o?: object) => Promise<void> } | HTMLElement | null): Promise<HTMLElement[]> {
  if (!trigger) throw new Error('No select trigger')
  if (trigger instanceof HTMLElement) {
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  } else {
    await trigger.trigger('keydown', { key: 'Enter' })
  }
  await flushPromises()
  return selectOptions()
}

/** 关闭已打开的真实 Select（Esc）。 */
export async function closeSelect(): Promise<void> {
  document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  await flushPromises()
}

/**
 * 打开真实 reka Select 并按显示文本选中一项。
 * 触发器用键盘 Enter 打开、选项用键盘 Enter 选中：reka 的 SelectItem 监听 pointerup，
 * jsdom 的 PointerEvent 不完整，键盘路径是同一个 handleSelect，且不依赖指针事件。
 */
export async function pickSelectOption(
  trigger: { trigger: (e: string, o?: object) => Promise<void> } | HTMLElement | null,
  label: string
) {
  if (!trigger) throw new Error('No select trigger')
  if (trigger instanceof HTMLElement) {
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  } else {
    await trigger.trigger('keydown', { key: 'Enter' })
  }
  await flushPromises()
  const option = selectOptions().find((o) => o.textContent?.trim() === label)
  if (!option) throw new Error(`No select option "${label}". options=${selectOptions().map((o) => o.textContent?.trim()).join(', ')}`)
  option.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  await flushPromises()
}

/**
 * 打开真实 Popover：点触发器。同一页面有多个 Popover 时用 `scope` 限定：容器选择器
 * （如 `[data-widget="kpi-total"]`）或触发器自身的选择器（如 `[data-sidebar-group-add="general"]`）。已打开则不重复点击。
 */
export async function openPopover(wrapper: VueWrapper, scope = ''): Promise<void> {
  // scope 可以是包含触发器的容器，也可以是触发器本身（as-child 时自定义属性落在触发器上）
  const inside = wrapper.find(`${scope} [data-slot="popover-trigger"]`.trim())
  const self = scope ? wrapper.find(`${scope}[data-slot="popover-trigger"]`) : inside
  const trigger = inside.exists() ? inside : self
  if (!trigger.exists()) throw new Error(`No popover trigger for "${scope}"`)
  if (trigger.attributes('aria-expanded') !== 'true') await trigger.trigger('click')
  await flushPromises()
}

/** 弹层内容 teleport 在 body 里：取元素（找不到返回 null）。 */
export function popoverEl<T extends HTMLElement = HTMLElement>(selector: string): T | null {
  return document.body.querySelector<T>(selector)
}

/** 点击弹层内容里的元素（找不到直接抛，避免静默跳过）。 */
export async function clickInPopover(selector: string): Promise<void> {
  const el = popoverEl(selector)
  if (!el) throw new Error(`No element "${selector}" in popover content`)
  el.click()
  await flushPromises()
}

/** 打开弹层再点其中一项：最常见的「更多菜单」操作。 */
export async function chooseFromPopover(wrapper: VueWrapper, itemSelector: string, scope = ''): Promise<void> {
  await openPopover(wrapper, scope)
  await clickInPopover(itemSelector)
}

/** 真实 Popover + Command（弹层内容 teleport 到 body，故关闭 teleport 桩并挂到 document）。 */
export const realPopoverStubs = {
  teleport: false,
  Button: false,
  Popover: false,
  PopoverTrigger: false,
  PopoverContent: false,
  Command: false,
  CommandInput: false,
  CommandList: false,
  CommandEmpty: false,
  CommandGroup: false,
  CommandItem: false
}

/** 真实 SbModal + reka Dialog：内容 teleport 到 body；需要 attachTo: document.body。 */
export const realDialogStubs = {
  teleport: false,
  SbModal: false,
  Button: false,
  Dialog: false,
  DialogContent: false,
  DialogHeader: false,
  DialogTitle: false,
  DialogDescription: false,
  DialogFooter: false
}

/** 当前打开的 Dialog 内容（没有打开返回 null）。 */
export function dialogEl(): HTMLElement | null {
  // 叠开多层对话框（如「列表」之上再开「编辑」）时，操作对象永远是最上层那个。
  const all = document.body.querySelectorAll<HTMLElement>('[role="dialog"]')
  return all.length ? all[all.length - 1] : null
}

/**
 * 以当前打开的对话框（没有则 body）为根的 DOMWrapper：保留 `.get / .find / .findAll / .text / .html /
 * .setValue / .trigger` 的写法，用来读写 teleport 到 body 里的真实 Dialog 内容。
 */
export function inDialog(): DOMWrapper<HTMLElement> {
  return new DOMWrapper(dialogEl() ?? document.body)
}

/**
 * 等真实 Dialog 的内容挂到 body（reka 的 Portal 在下一个 tick 才渲染），
 * 让调用方可以马上同步断言对话框内部结构。
 */
export async function waitDialog(): Promise<void> {
  for (let i = 0; i < 5 && !dialogEl(); i++) await flushPromises()
}

/**
 * 挂载含 SbModal / Dialog 的组件并等真实对话框渲染出来（`open` 为真时）。
 * 等价于 `mount(..., { attachTo: document.body })` + 等一次 Portal 挂载，调用方可同步断言对话框内部。
 */
export async function mountDialog<T>(component: T, options: Record<string, unknown> = {}): Promise<VueWrapper> {
  const wrapper = mount(component as never, { attachTo: document.body, ...options } as never) as unknown as VueWrapper
  await waitDialog()
  return wrapper
}

/** 当前打开的对话框标题文本（没有打开返回空串）。 */
export function dialogTitle(): string {
  return document.body.querySelector('[data-slot="dialog-title"]')?.textContent?.trim() ?? ''
}

/** 当前打开的 SbModal 页脚按钮：`[取消, 确定]`（自定义 footer 时是自定义按钮）。 */
function modalFooterButtons(): HTMLButtonElement[] {
  const dialog = dialogEl()
  if (!dialog) return []
  const footer = dialog.querySelector<HTMLElement>('[data-slot="dialog-footer"]')
  return footer ? Array.from(footer.querySelectorAll<HTMLButtonElement>('button')) : []
}

/** SbModal 页脚最后一个按钮（即「确定 / 保存 / 开始」），找不到返回 null。 */
export function modalFooterLast(): HTMLButtonElement | null {
  return modalFooterButtons().at(-1) ?? null
}

/**
 * 点 SbModal 的确认按钮：默认取页脚最后一个（SbModal 的「确定」在「取消」之后）。
 * 按钮被禁用时抛错——真实用户点不了，测试也不该点过去。
 */
export async function okModal(): Promise<void> {
  const footer = modalFooterButtons()
  const button = footer.at(-1)
  if (!button) throw new Error('No confirm button in the open modal')
  const label = button.textContent?.trim() ?? ''
  if (button.disabled) throw new Error(`Confirm button "${label}" is disabled — a real user could not click it`)
  button.click()
  await flushPromises()
}

/** 点 SbModal 的取消按钮（页脚第一个）。 */
export async function cancelModal(): Promise<void> {
  const button = modalFooterButtons()[0]
  if (!button) throw new Error('No cancel button in the open modal')
  button.click()
  await flushPromises()
}

/** 点 SbModal 页脚里文字为 label 的按钮（自定义文案 / 自定义 footer 时用）。 */
export async function clickModalButton(label: string): Promise<void> {
  const dialog = dialogEl()
  const buttons = dialog ? Array.from(dialog.querySelectorAll<HTMLButtonElement>('button')) : []
  const button = buttons.find((b) => b.textContent?.trim() === label)
  if (!button) throw new Error(`No "${label}" button in the open modal. buttons=${buttons.map((b) => b.textContent?.trim()).join(' | ')}`)
  if (button.disabled) throw new Error(`"${label}" is disabled — a real user could not click it`)
  button.click()
  await flushPromises()
}

/** SbModal 的确认按钮当前是否被禁用（没有打开返回 null）。 */
export function modalOkDisabled(): boolean | null {
  const button = modalFooterButtons().at(-1)
  return button ? button.disabled : null
}

/** 点击真实 Dialog 右上角的关闭按钮。 */
export async function closeDialog(): Promise<void> {
  const button = document.body.querySelector<HTMLElement>('[data-slot="dialog-close"]')
  if (!button) throw new Error('No dialog close button')
  button.click()
  await flushPromises()
}

/** 真实 AlertDialog（ConfirmAction 的内核）：内容 teleport 到 body，确认 / 取消是真实按钮。 */
export const realAlertDialogStubs = {
  teleport: false,
  ConfirmAction: false,
  Button: false,
  AlertDialog: false,
  AlertDialogTrigger: false,
  AlertDialogContent: false,
  AlertDialogHeader: false,
  AlertDialogTitle: false,
  AlertDialogDescription: false,
  AlertDialogFooter: false,
  AlertDialogCancel: false,
  AlertDialogAction: false
}

/**
 * 当前**打开**的 AlertDialog 内容（没有打开返回 null）。
 *
 * - reka 的 content 节点上 data-slot / role 会被外层透传属性覆盖，所以按稳定的子结构反推：
 *   取同时包含标题与页脚的最近公共祖先。
 * - 同一页面可能有多个 ConfirmAction（受控的关闭时内容仍挂在 DOM 里），要找的是**打开中**那个：
 *   打开时 reka 会给内容所在的 portal 同时渲染一个 `data-state="open"` 的遮罩层，据此判定。
 */
export function alertDialogEl(): HTMLElement | null {
  const overlays = Array.from(document.body.querySelectorAll<HTMLElement>('[data-slot="alert-dialog-overlay"][data-state="open"]'))
  if (!overlays.length) return null
  const footers = Array.from(document.body.querySelectorAll<HTMLElement>('[data-slot="alert-dialog-footer"]'))
  // 内容与遮罩是兄弟节点（遮罩在前）：取遮罩之后第一个包含页脚的兄弟
  for (const overlay of overlays.slice().reverse()) {
    for (let sib = overlay.nextElementSibling as HTMLElement | null; sib; sib = sib.nextElementSibling as HTMLElement | null) {
      if (footers.some((f) => sib!.contains(f))) return sib
    }
  }
  return null
}

/** 点击真实 AlertDialog 里文字为 label 的按钮（默认「确定」；取消用「取消」）。 */
export async function clickAlertDialog(label = '确定'): Promise<void> {
  const dialog = alertDialogEl()
  if (!dialog) throw new Error('No open alert dialog')
  const button = Array.from(dialog.querySelectorAll<HTMLElement>('button')).find((b) => b.textContent?.trim() === label)
  if (!button) throw new Error(`No "${label}" button in alert dialog`)
  button.click()
  await flushPromises()
}

/**
 * 走完一次真实 ConfirmAction：先点触发按钮（文字为 triggerText，在 root 内找；找不到时在整个文档找），
 * 再点弹出的确认框里的「确定」。triggerIndex 用于同文案按钮出现多次的页面（默认取第一个）。
 */
export async function confirmAction(
  root: Pick<VueWrapper, 'findAll'> | DOMWrapper<HTMLElement>,
  triggerText: string,
  opts: { index?: number; button?: string } = {}
): Promise<void> {
  const selector = 'button, [role="button"]'
  const inRoot = root.findAll(selector).filter((n) => n.text().trim() === triggerText)
  const nodes = inRoot.length ? inRoot : new DOMWrapper(document.body).findAll(selector).filter((n) => n.text().trim() === triggerText)
  const idx = opts.index ?? 0
  const trigger = idx < 0 ? nodes.at(idx) : nodes[idx]
  if (!trigger) throw new Error(`No "${triggerText}" trigger for ConfirmAction (found ${nodes.length})`)
  await trigger.trigger('click')
  await flushPromises()
  await clickAlertDialog(opts.button ?? '确定')
}

/** 真实 reka Select：触发器是 `button[role=combobox]`，选项 teleport 到 body（teleport: false 后在 DOM 里）。 */
export const realSelectStubs = {
  teleport: false,
  Select: false,
  SelectTrigger: false,
  SelectValue: false,
  SelectContent: false,
  SelectGroup: false,
  SelectItem: false
}

/** 页面里第 index 个真实 Select 触发器（按文档顺序）。 */
export function selectTriggers(root: { findAll: (s: string) => Array<{ element: Element }> } | ParentNode = document): HTMLElement[] {
  if ('findAll' in root) return root.findAll('button[role="combobox"]').map((w) => w.element as HTMLElement)
  return Array.from(root.querySelectorAll<HTMLElement>('button[role="combobox"]'))
}

/** 真实 ToggleGroup / ToggleGroupItem：渲染为带 data-state 的按钮。 */
export const realToggleStubs = { ToggleGroup: false, ToggleGroupItem: false }

/**
 * 点击真实 reka TabsTrigger：它响应的是左键 mousedown（见 reka-ui TabsTrigger），不是 click。
 * 按 `value`（页签 id）定位：`data-tab` 之类的钩子不属于真实组件契约，这里用 aria 关系找。
 */
export async function clickTab(wrapper: Pick<VueWrapper, 'findAll'> | DOMWrapper<HTMLElement>, value: string) {
  // wrapper 内找不到时（页签在 teleport 到 body 的弹窗里）回退到整个文档
  const inWrapper = wrapper.findAll('[role="tab"]')
  const tabs = inWrapper.length ? inWrapper : new DOMWrapper(document.body).findAll('[role="tab"]')
  const trigger = tabs.find((t) => t.attributes('id')?.endsWith(`-trigger-${value}`))
  if (!trigger) {
    throw new Error(`No tab "${value}". tabs=${tabs.map((t) => t.attributes('id')).join(', ')}`)
  }
  await trigger.trigger('mousedown', { button: 0 })
}

export function clickText(wrapper: Pick<VueWrapper, 'findAll' | 'text'> | DOMWrapper<HTMLElement>, text: string) {
  const selector = 'button, a, [role="button"]'
  // wrapper 内没有时（按钮在 teleport 到 body 的弹窗里）回退到整个文档
  const nodes = wrapper.findAll(selector).filter((n) => n.text().includes(text))
  const el = nodes[0] ?? new DOMWrapper(document.body).findAll(selector).find((n) => n.text().includes(text))
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

export const creatingDb = {
  id: 'db-2',
  name: 'creating-db',
  status: 'creating' as const,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z'
}

export const degradedDb = {
  id: 'db-deg',
  name: 'degraded-db',
  status: 'degraded' as const,
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
