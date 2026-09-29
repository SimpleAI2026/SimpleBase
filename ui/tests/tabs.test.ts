import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent } from 'vue'
import { tabsListVariants } from '.'
import Tabs from './Tabs.vue'
import TabsContent from './TabsContent.vue'
import TabsList from './TabsList.vue'
import TabsTrigger from './TabsTrigger.vue'

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
    expect(wrapper.text()).toContain('panel')
  })
})
