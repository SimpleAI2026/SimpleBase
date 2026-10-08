import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

/**
 * ui/ 基础组件的状态属性回归（planv5.0 §2.1 / §7.3）。
 *
 * 这些用例刻意**不桩** ui 组件：锁定版本的 reka-ui（2.10.5）渲染的是带值的
 * `data-state`（checked/unchecked、active/inactive、open/closed），而组件里
 * 一旦误写成裸 `data-checked:` / `data-open:` 之类的 Tailwind 变体，编译出的
 * 选择器 `[data-checked]` / `[data-open]` 永不命中 —— 表现为「开关点了没反应」
 * 且无任何报错。这里断言真实 DOM 属性与组件契约，锁死该缺陷不复发。
 *
 * 注意：`data-active` 是个例外 —— reka 的 RovingFocusItem 会渲染空属性
 * （node_modules/reka-ui/dist/RovingFocus/RovingFocusItem.js:82），
 * 所以 TabsTrigger 上的 `data-active:` 变体本来就能命中。
 */

describe('ui state attributes', () => {
  it('Switch exposes data-state and honours the reka-ui 2.10.5 modelValue contract', async () => {
    const wrapper = mount(
      defineComponent({
        components: { Switch },
        template: '<Switch :model-value="on" @update:model-value="on = $event" />',
        setup() {
          const on = ref(false)
          return { on }
        }
      }),
      { attachTo: document.body }
    )

    const root = wrapper.find('button')
    expect(root.attributes('data-state')).toBe('unchecked')
    expect(root.attributes('aria-checked')).toBe('false')
    // 旧实现依赖的裸属性 reka 从不渲染，样式钩子必然失效
    expect(root.attributes('data-checked')).toBeUndefined()
    expect(root.attributes('data-unchecked')).toBeUndefined()

    await root.trigger('click')
    await nextTick()

    expect(root.attributes('data-state')).toBe('checked')
    expect(root.attributes('aria-checked')).toBe('true')
    expect((wrapper.vm as unknown as { on: boolean }).on).toBe(true)

    wrapper.unmount()
  })

  it('Switch 受控关闭时点击会向上抛出 update:modelValue', async () => {
    const received: boolean[] = []
    const wrapper = mount(
      defineComponent({
        components: { Switch },
        template: '<Switch :model-value="true" @update:model-value="onToggle" />',
        setup() {
          return { onToggle: (v: boolean) => received.push(v) }
        }
      }),
      { attachTo: document.body }
    )

    await wrapper.find('button').trigger('click')
    expect(received).toEqual([false])

    wrapper.unmount()
  })

  it('Switch thumb carries the same data-state used by the translate hook', async () => {
    const wrapper = mount(
      defineComponent({
        components: { Switch },
        template: '<Switch :model-value="true" />'
      }),
      { attachTo: document.body }
    )

    const thumb = wrapper.find('[data-slot=switch-thumb]')
    expect(thumb.exists()).toBe(true)
    expect(thumb.attributes('data-state')).toBe('checked')

    wrapper.unmount()
  })

  it('TabsTrigger exposes data-state active/inactive for the active-style hook', async () => {
    const wrapper = mount(
      defineComponent({
        components: { Tabs, TabsList, TabsTrigger },
        template:
          '<Tabs default-value="a"><TabsList><TabsTrigger value="a">A</TabsTrigger><TabsTrigger value="b">B</TabsTrigger></TabsList></Tabs>'
      }),
      { attachTo: document.body }
    )

    const [a, b] = wrapper.findAll('button')
    expect(a.attributes('data-state')).toBe('active')
    expect(b.attributes('data-state')).toBe('inactive')
    // RovingFocusItem 给选中项渲染空的 data-active，两条钩子对选中态等价
    expect(a.attributes('data-active')).toBe('')
    expect(b.attributes('data-active')).toBeUndefined()

    wrapper.unmount()
  })
})
