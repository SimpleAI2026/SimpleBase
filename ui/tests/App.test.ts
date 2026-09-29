import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import { useSettingsStore } from './stores/settings'

describe('App', () => {
  it('applies theme on mount and reacts to system/theme changes', async () => {
    const listeners: Array<() => void> = []
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: false,
        media: query,
        addEventListener: (_: string, cb: () => void) => listeners.push(cb),
        removeEventListener: () => undefined
      })
    })
    setActivePinia(createPinia())
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div>home</div>' } }]
    })
    await router.push('/')
    await router.isReady()
    const w = mount(App, {
      global: {
        plugins: [router, createPinia()],
        stubs: {
          TooltipProvider: { template: '<div><slot /></div>' },
          Toaster: { template: '<div />' },
          Sonner: { template: '<div />' }
        }
      }
    })
    await flushPromises()
    expect(w.text()).toContain('home')
    const store = useSettingsStore()
    store.setTheme('system')
    listeners.forEach((fn) => fn())
    store.setTheme('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    w.unmount()
  })
})
