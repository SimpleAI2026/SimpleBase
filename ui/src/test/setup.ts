import { config } from '@vue/test-utils'
import { beforeEach, vi } from 'vitest'
import { i18n, setI18nLocale } from '@/locales'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false
    })
  })
}

class MemoryStorage implements Storage {
  private store = new Map<string, string>()
  get length() {
    return this.store.size
  }
  clear() {
    this.store.clear()
  }
  getItem(key: string) {
    return this.store.has(key) ? this.store.get(key)! : null
  }
  key(index: number) {
    return Array.from(this.store.keys())[index] ?? null
  }
  removeItem(key: string) {
    this.store.delete(key)
  }
  setItem(key: string, value: string) {
    this.store.set(key, String(value))
  }
}

const memory = new MemoryStorage()
Object.defineProperty(globalThis, 'localStorage', { value: memory, configurable: true })
Object.defineProperty(window, 'localStorage', { value: memory, configurable: true })

vi.mock('vue-sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn()
  },
  Toaster: { name: 'Toaster', template: '<div class="toaster" />' }
}))

// jsdom 无 canvas 2d，echarts 需 mock（各用例可再 mock 断言 setOption）
vi.mock('echarts/core', () => {
  const make = () => ({
    setOption: vi.fn(),
    resize: vi.fn(),
    dispose: vi.fn(),
    on: vi.fn(),
    off: vi.fn(),
    getOption: vi.fn(() => ({}))
  })
  return {
    init: vi.fn(make),
    use: vi.fn(),
    default: { init: vi.fn(make), use: vi.fn() }
  }
})
vi.mock('echarts/charts', () => ({ BarChart: {}, LineChart: {} }))
vi.mock('echarts/components', () => ({ GridComponent: {}, TooltipComponent: {} }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))
vi.mock('echarts', () => {
  const make = () => ({
    setOption: vi.fn(),
    resize: vi.fn(),
    dispose: vi.fn(),
    on: vi.fn(),
    off: vi.fn(),
    getOption: vi.fn(() => ({}))
  })
  return {
    init: vi.fn(make),
    default: { init: vi.fn(make) }
  }
})

config.global.plugins = [i18n]
config.global.stubs = {
  teleport: true,
  Transition: false,
  RouterLink: { template: '<a><slot /></a>' }
}

// 组件外（services / utils / composables / store getter）走 i18n.global.t，
// 与这里的实例是同一个，所以固定 zh-CN 后既有中文断言全部成立。
setI18nLocale('zh-CN')

beforeEach(() => {
  memory.clear()
  document.documentElement.className = ''
  document.documentElement.removeAttribute('data-theme')
  // 语言是全局单例状态：单个用例切到 en 后必须复位，否则污染同文件后续用例。
  setI18nLocale('zh-CN')
  document.documentElement.lang = 'zh-CN'
})
