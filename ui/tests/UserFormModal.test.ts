import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { useAuthStore } from '@/stores/auth'
import UserFormModal from '@/components/modal/UserFormModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const adminUser = {
  id: 'u-self', username: 'root', role: 'superadminl1', displayName: 'Root',
  email: '', status: 'active', mustChangePassword: false
}

const existingUser = {
  id: 'u-9', username: 'ada', role: 'admin', displayName: 'Ada',
  status: 'active', createdAt: '2026-01-01T00:00:00Z', lastLoginAt: ''
}

function setup(props: { open: boolean; user: unknown }) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.applyTokens({ accessToken: 'a', refreshToken: 'r', user: adminUser })
  const w = mount(UserFormModal, {
    props: props as never,
    global: { plugins: [pinia], stubs: uiStubs }
  })
  return { auth, w }
}

describe('UserFormModal', () => {
  beforeEach(() => resetApiMocks())

  it('新建用户：填表提交并转发 saved', async () => {
    const { w } = setup({ open: true, user: null })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.title).toContain('新建')
    // canSubmit 守卫：空表单不可提交
    expect(vm.canSubmit).toBe(false)

    // 用户名 / 密码 Input 桩的 v-model 回调
    await w.get('#uf-username').setValue('carol')
    await w.get('#uf-password').setValue('12345678')
    // 角色 Select 桩 emit
    await w.get('.select-emit').trigger('click')
    await flushPromises()
    expect(vm.canSubmit).toBe(true)

    await vm.submit()
    await flushPromises()
    expect(api.users.create).toHaveBeenCalledWith(
      expect.objectContaining({ username: 'carol', password: '12345678', role: 'info' })
    )
    expect(toast.success).toHaveBeenCalled()
    expect(w.emitted('saved')).toBeTruthy()
    w.unmount()
  })

  it('新建失败展示错误', async () => {
    api.users.create.mockRejectedValueOnce(new Error('用户名已存在'))
    const { w } = setup({ open: true, user: null })
    await flushPromises()
    const vm = w.vm as any
    vm.username = 'carol'
    vm.password = '12345678'
    vm.role = 'admin'
    await vm.submit()
    await flushPromises()
    expect(w.text()).toContain('用户名已存在')
    w.unmount()
  })

  it('编辑用户：更新资料并重置密码', async () => {
    // watch(open) immediate 才会填充表单：先 false 再 true
    const { w } = setup({ open: false, user: existingUser })
    await w.setProps({ open: true })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.title).toContain('编辑')
    expect(vm.username).toBe('ada')
    // 编辑态 canSubmit 恒真
    expect(vm.canSubmit).toBe(true)
    vm.displayName = 'Ada L.'
    vm.password = 'newpassword'
    await vm.submit()
    await flushPromises()
    expect(api.users.update).toHaveBeenCalledWith('u-9', {
      displayName: 'Ada L.', role: 'admin', password: 'newpassword'
    })
    expect(w.emitted('saved')).toBeTruthy()
    w.unmount()
  })

  it('SbModal close 转发 update:open', async () => {
    const { w } = setup({ open: true, user: null })
    await w.get('.sb-modal-close').trigger('click')
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })
})
