import { describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useCanWrite } from '@/composables/useCanWrite'
import { useAuthStore } from '@/stores/auth'
import { ADMIN_PROJECT_ID, useProjectStore } from '@/stores/project'

function setup() {
  setActivePinia(createPinia())
  return { auth: useAuthStore(), project: useProjectStore(), canWrite: useCanWrite() }
}

describe('useCanWrite：写入口 = 非系统项目 && 角色可写', () => {
  it('未登录（API Key 通道）在普通项目可写，在系统项目不可写', () => {
    const { project, canWrite } = setup()
    project.setProject('p1', 'P')
    expect(canWrite.value).toBe(true)
    project.setProject(ADMIN_PROJECT_ID, 'Admin')
    expect(canWrite.value).toBe(false)
  })

  it('登录态：superadminl1 / user 可写，admin（只读管理员）不可写', () => {
    const { auth, project, canWrite } = setup()
    project.setProject('p1', 'P')
    for (const [role, expected] of [['superadminl1', true], ['user', true], ['admin', false]] as const) {
      auth.user = { id: 'u', username: 'x', role } as never
      expect(canWrite.value, role).toBe(expected)
    }
  })

  it('即使角色可写，系统项目里也不可写（前端体验层，后端另有保护）', () => {
    const { auth, project, canWrite } = setup()
    auth.user = { id: 'u', username: 'x', role: 'superadminl1' } as never
    project.setProject(ADMIN_PROJECT_ID, 'Admin')
    expect(canWrite.value).toBe(false)
    project.setProject('p1', 'P')
    expect(canWrite.value).toBe(true)
  })
})
