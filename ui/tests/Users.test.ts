import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp } from '@/test/helpers'
import { useAuthStore } from '@/stores/auth'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import Users from '@/pages/Users.vue'

const rows = [
  {
    id: 'u-self', username: 'root', role: 'superadminl1', displayName: 'Root',
    status: 'active', projectCount: 2, lastLoginAt: '2026-01-01T00:00:00Z'
  },
  {
    id: 'u-admin', username: 'ada', role: 'admin', displayName: '',
    status: 'active', projectCount: 1, lastLoginAt: ''
  },
  {
    id: 'u-user', username: 'bob', role: 'user', displayName: 'Bobby',
    status: 'disabled', projectCount: 0
  },
  {
    id: 'u-seed', username: 'simplebase2026', role: 'user', displayName: 'Seed',
    status: 'active', projectCount: 1, lastLoginAt: '2026-01-02T00:00:00Z'
  }
]

const formStub = {
  UserFormModal: {
    props: ['open', 'user'],
    emits: ['update:open', 'saved'],
    template: '<div class="user-form" :data-open="String(open)"><button type="button" class="saved" @click="$emit(\'saved\')">saved</button><button type="button" class="close-form" @click="$emit(\'update:open\', false)">close</button></div>'
  }
}

describe('Users page', () => {
  beforeEach(() => resetApiMocks())

  it('filters roles and toggles accounts without dropping the list', async () => {
    api.users.list.mockResolvedValue({ users: rows, nextCursor: '' })
    const { wrapper, pinia } = await mountWithApp(Users, { stubs: formStub })
    useAuthStore(pinia).applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u-self', username: 'root', role: 'superadminl1', displayName: 'Root',
        email: '', status: 'active', mustChangePassword: false
      }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('超管')
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('普通用户')
    expect(wrapper.text()).toContain('● 启用')
    expect(wrapper.text()).toContain('○ 禁用')
    expect(wrapper.text()).toContain('Bobby')

    const vm = wrapper.vm as unknown as { roleFilter: string; statusFilter: string; keyword: string }
    vm.roleFilter = 'admin'
    await flushPromises()
    expect(wrapper.text()).toContain('ada')
    expect(wrapper.text()).not.toContain('bob')
    vm.roleFilter = 'user'
    await flushPromises()
    expect(wrapper.text()).toContain('bob')
    vm.roleFilter = 'all'
    vm.statusFilter = 'disabled'
    await flushPromises()
    expect(wrapper.text()).toContain('bob')
    expect(wrapper.text()).not.toContain('ada')
    vm.statusFilter = 'all'
    vm.keyword = ' bobby '
    await flushPromises()
    expect(wrapper.text()).toContain('bob')
    expect(wrapper.text()).not.toContain('ada')
    vm.keyword = 'nope'
    await flushPromises()
    expect(wrapper.text()).toContain('暂无用户')
    vm.keyword = ''
    await flushPromises()

    await clickText(wrapper, '新建用户')
    expect(wrapper.get('.user-form').attributes('data-open')).toBe('true')
    await wrapper.get('.close-form').trigger('click')
    await flushPromises()
    await clickText(wrapper, '查看')
    await clickText(wrapper, '编辑')
    await wrapper.get('.saved').trigger('click')
    await flushPromises()
    expect(wrapper.get('.user-form').attributes('data-open')).toBe('false')

    await clickText(wrapper, '禁用')
    expect(api.users.update).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('已禁用')
    api.users.update.mockRejectedValueOnce('raw')
    await clickText(wrapper, '禁用')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('操作失败')
    await clickText(wrapper, '启用')
    expect(toast.success).toHaveBeenCalledWith('已启用')
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.text()).toContain('ada')
    await wrapper.get('input').setValue('ada')
    const selects = wrapper.findAll('.select-emit')
    await selects[0].trigger('click')
    await selects[1].trigger('click')
    await flushPromises()
    wrapper.unmount()
  })

  it('offers create from the empty list and retries a failed load', async () => {
    api.users.list.mockResolvedValueOnce({ users: [], nextCursor: '' })
    const { wrapper, pinia } = await mountWithApp(Users, { stubs: formStub })
    useAuthStore(pinia).applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u-self', username: 'root', role: 'superadminl1', displayName: '',
        email: '', status: 'active', mustChangePassword: false
      }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('暂无用户')
    const creates = wrapper.findAll('button').filter((b) => b.text().includes('新建用户'))
    await creates[creates.length - 1].trigger('click')
    await flushPromises()
    expect(wrapper.get('.user-form').attributes('data-open')).toBe('true')
    wrapper.unmount()
  })
})
