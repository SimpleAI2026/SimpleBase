import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, mountWithApp, installJsdomPolyfills, pickSelectOption, realSelectStubs, selectTriggers, closeDialog, realDialogStubs, waitDialog, inDialog } from '@/test/helpers'
import { useAuthStore } from '@/stores/auth'
import UserFormModal from '@/components/modal/UserFormModal.vue'
import type { AuthUser } from '@/services/types'
import LoginModal from '@/components/modal/LoginModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const adminUser: AuthUser = {
  id: 'u-self', username: 'root', role: 'superadminl1', displayName: 'Root',
  email: '', status: 'active', mustChangePassword: false
}

const existingUser = {
  id: 'u-9', username: 'ada', role: 'admin', displayName: 'Ada',
  status: 'active', createdAt: '2026-01-01T00:00:00Z', lastLoginAt: ''
}

async function setup(props: { open: boolean; user: unknown }) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.applyTokens({ accessToken: 'a', refreshToken: 'r', user: adminUser })
  const w = mount(UserFormModal, {
    attachTo: document.body,
    props: props as never,
    global: { plugins: [pinia], stubs: { ...uiStubs, ...realDialogStubs, ...realSelectStubs } }
  })
  await waitDialog()
  return { auth, w }
}

installJsdomPolyfills()

describe('UserFormModal', () => {
  beforeEach(() => resetApiMocks())

  it('新建用户：填表提交并转发 saved', async () => {
    const { w } = await setup({ open: true, user: null })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.title).toContain('新建')
    // canSubmit 守卫：空表单不可提交
    expect(vm.canSubmit).toBe(false)

    // 用户名 / 密码 Input 桩的 v-model 回调
    await inDialog().get('#uf-username').setValue('carol')
    await inDialog().get('#uf-password').setValue('12345678')
    // 真实 Select：角色选「管理员」
    await pickSelectOption(selectTriggers()[0], '管理员（只读）')
    await flushPromises()
    expect(vm.canSubmit).toBe(true)

    await vm.submit()
    await flushPromises()
    expect(api.users.create).toHaveBeenCalledWith(
      expect.objectContaining({ username: 'carol', password: '12345678', role: 'admin' })
    )
    expect(toast.success).toHaveBeenCalled()
    expect(w.emitted('saved')).toBeTruthy()
    w.unmount()
  })

  it('新建失败展示错误', async () => {
    api.users.create.mockRejectedValueOnce(new Error('用户名已存在'))
    const { w } = await setup({ open: true, user: null })
    await flushPromises()
    const vm = w.vm as any
    vm.username = 'carol'
    vm.password = '12345678'
    vm.role = 'admin'
    await vm.submit()
    await flushPromises()
    expect(inDialog().text()).toContain('用户名已存在')
    w.unmount()
  })

  it('编辑用户：更新资料并重置密码', async () => {
    // watch(open) immediate 才会填充表单：先 false 再 true
    const { w } = await setup({ open: false, user: existingUser })
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
    const { w } = await setup({ open: true, user: null })
    await closeDialog()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })
})

describe('UserFormModal 分支', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('creates a user when role and password valid', async () => {
    const { wrapper, pinia } = await mountWithApp(UserFormModal, {
      props: { open: true, user: null },
      stubs: realDialogStubs
    })
    const { useAuthStore } = await import('@/stores/auth')
    useAuthStore(pinia).applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u1',
        username: 'simplebase2026',
        role: 'superadminl1',
        displayName: '',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    await flushPromises()
    const inputs = inDialog().findAll('input')
    await inputs[0].setValue('alice')
    await inputs[1].setValue('Alice')
    await inputs[2].setValue('12345678')
    // 触发 create：canSubmit 需要 role，直接调用 submit 按钮
    const createBtn = inDialog().findAll('button').find((b) => b.text().includes('创建'))
    expect(createBtn).toBeTruthy()
    // role 为空时 disabled —— 设置 role 后再点
    ;(wrapper.vm as unknown as { role: string }).role = 'user'
    await flushPromises()
    await createBtn!.trigger('click')
    await flushPromises()
    expect(api.users.create).toHaveBeenCalled()
  })

  it('updates existing user and handles error', async () => {
    api.users.update.mockRejectedValueOnce(new Error('bad'))
    const user = {
      id: 'u2',
      username: 'bob',
      role: 'user' as const,
      displayName: 'B',
      email: '',
      status: 'active' as const,
      mustChangePassword: false,
      projectCount: 1
    }
    const { wrapper, pinia } = await mountWithApp(UserFormModal, {
      props: { open: true, user },
      stubs: realDialogStubs
    })
    const { useAuthStore } = await import('@/stores/auth')
    useAuthStore(pinia).applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u1',
        username: 'simplebase2026',
        role: 'superadminl1',
        displayName: '',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    await flushPromises()
    const saveBtn = inDialog().findAll('button').find((b) => b.text().includes('保存'))
    await saveBtn!.trigger('click')
    await flushPromises()
    expect(api.users.update).toHaveBeenCalled()
  })
})
