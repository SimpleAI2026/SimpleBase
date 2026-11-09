import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent } from 'vue'
import { tabsListVariants } from '@/components/ui/tabs'
import Tabs from '@/components/ui/tabs/Tabs.vue'
import TabsContent from '@/components/ui/tabs/TabsContent.vue'
import TabsList from '@/components/ui/tabs/TabsList.vue'
import TabsTrigger from '@/components/ui/tabs/TabsTrigger.vue'

describe('tabs orientation classes', () => {
  it('matches data-orientation on the tabs group, not boolean data-horizontal', () => {
    const classes = tabsListVariants()
    expect(classes).toContain('group-data-[orientation=horizontal]/tabs:h-8')
    expect(classes).toContain('group-data-[orientation=vertical]/tabs:h-fit')
    expect(classes).toContain('group-data-[orientation=vertical]/tabs:flex-col')
    expect(classes).not.toContain('group-data-horizontal/tabs:')
    expect(classes).not.toContain('group-data-vertical/tabs:')
  })

  it('stacks the tab list above content for the default horizontal orientation', () => {
    const Harness = defineComponent({
      components: { Tabs, TabsList, TabsTrigger, TabsContent },
      template: `
        <Tabs default-value="a">
          <TabsList>
            <TabsTrigger value="a">A</TabsTrigger>
          </TabsList>
          <TabsContent value="a">panel</TabsContent>
        </Tabs>
      `
    })
    const wrapper = mount(Harness)
    const root = wrapper.get('[data-slot="tabs"]')
    expect(root.attributes('data-orientation')).toBe('horizontal')
    expect(root.classes()).toContain('data-[orientation=horizontal]:flex-col')
    expect(root.classes()).not.toContain('data-horizontal:flex-col')

    const list = wrapper.get('[data-slot="tabs-list"]')
    expect(list.classes()).toContain('group-data-[orientation=horizontal]/tabs:h-8')

    const trigger = wrapper.get('[data-slot="tabs-trigger"]')
    expect(trigger.classes()).toContain('group-data-[orientation=horizontal]/tabs:after:h-0.5')
    expect(trigger.classes()).toContain('group-data-[orientation=vertical]/tabs:w-full')
    expect(trigger.classes()).toContain('justify-center')
    expect(trigger.classes()).toContain('text-center')
    expect(trigger.classes().join(' ')).not.toContain('justify-start')
    expect(wrapper.text()).toContain('panel')
  })

  it('centers trigger labels in vertical, line, and flex-none layouts', () => {
    const Harness = defineComponent({
      components: { Tabs, TabsList, TabsTrigger },
      template: `
        <div>
          <Tabs orientation="vertical" default-value="a">
            <TabsList>
              <TabsTrigger value="a">纵向</TabsTrigger>
            </TabsList>
          </Tabs>
          <Tabs default-value="b">
            <TabsList variant="line" class="w-full justify-start">
              <TabsTrigger value="b" class="flex-none px-4">文档</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      `
    })
    const wrapper = mount(Harness)
    const triggers = wrapper.findAll('[data-slot="tabs-trigger"]')
    expect(wrapper.get('[data-slot="tabs"]').attributes('data-orientation')).toBe('vertical')
    const lists = wrapper.findAll('[data-slot="tabs-list"]')
    expect(lists[1].attributes('data-variant')).toBe('line')
    for (const trigger of triggers) {
      expect(trigger.classes()).toContain('justify-center')
      expect(trigger.classes()).toContain('text-center')
      expect(trigger.classes().join(' ')).not.toContain('justify-start')
    }
    expect(triggers[1].classes()).toContain('flex-none')
    expect(triggers[0].classes()).toContain('group-data-[orientation=vertical]/tabs:w-full')
  })
})
