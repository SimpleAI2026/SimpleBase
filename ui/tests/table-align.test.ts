import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent } from 'vue'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

describe('table alignment', () => {
  it('centers TableHead and TableCell, and lets text-left override the default', () => {
    const Harness = defineComponent({
      components: { Table, TableHeader, TableBody, TableRow, TableHead, TableCell },
      template: `
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>操作</TableHead>
              <TableHead class="text-left">消息</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell>
                <div class="flex gap-1"><button type="button">删除</button></div>
              </TableCell>
              <TableCell class="max-w-md text-left">long text</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      `
    })
    const wrapper = mount(Harness)
    const heads = wrapper.findAll('[data-slot="table-head"]')
    expect(heads[0].classes()).toContain('text-center')
    expect(heads[0].classes()).not.toContain('text-left')
    expect(heads[1].classes()).toContain('text-left')
    expect(heads[1].classes()).not.toContain('text-center')

    const cells = wrapper.findAll('[data-slot="table-cell"]')
    expect(cells[0].classes()).toContain('text-center')
    expect(cells[0].classes()).toContain('[&>.flex]:justify-center')
    expect(cells[1].classes()).toContain('text-left')
    expect(cells[1].classes()).not.toContain('text-center')
    expect(cells[1].classes()).toContain('max-w-md')
  })
})
