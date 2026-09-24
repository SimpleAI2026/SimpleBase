import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '../../test/api-mock'
import { mountWithApp } from '../../test/helpers'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

import UserFormModal from './UserFormModal.vue'
import LoginModal from './LoginModal.vue'

describe('UserFormModal + LoginModal branches', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('creates a user when role and password valid', async () => {
    const { wrapper, pinia } = await mountWithApp(UserFormModal, {
      props: { open: true, user: null }
    })
    const { useAuthStore } = await import('../../stores/auth')
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
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('alice')
    await inputs[1].setValue('Alice')
    await inputs[2].setValue('12345678')
    // 触发 create：canSubmit 需要 role，直接调用 submit 按钮
    const createBtn = wrapper.findAll('button').find((b) => b.text().includes('创建'))
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
      props: { open: true, user }
    })
    const { useAuthStore } = await import('../../stores/auth')
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
    const saveBtn = wrapper.findAll('button').find((b) => b.text().includes('保存'))
    await saveBtn!.trigger('click')
    await flushPromises()
    expect(api.users.update).toHaveBeenCalled()
  })

  it('LoginModal password change success path', async () => {
    api.auth.changePassword.mockResolvedValue(undefined)
    const { wrapper } = await mountWithApp(LoginModal, {
      props: { open: true, mode: 'password' }
    })
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('old12345')
    await inputs[1].setValue('new123456')
    await inputs[2].setValue('new123456')
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.auth.changePassword).toHaveBeenCalled()
  })

  it('LoginModal password change failure and sign out', async () => {
    api.auth.changePassword.mockRejectedValue(new Error('nope'))
    api.auth.logout = vi.fn(async () => undefined)
    const { wrapper } = await mountWithApp(LoginModal, {
      props: { open: true, mode: 'password' }
    })
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('old12345')
    await inputs[1].setValue('new123456')
    await inputs[2].setValue('new123456')
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('修改失败')
    const out = wrapper.findAll('button').find((b) => b.text().includes('退出'))
    await out!.trigger('click')
    await flushPromises()
  })
})
