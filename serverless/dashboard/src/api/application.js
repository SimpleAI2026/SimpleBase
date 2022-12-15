import store from '@/store'
import request from '@/utils/request'
import { setCurrentAppid } from "./index"

/**
 * 获取应用的协作者列表
 */
export function getCollaborators() {
  const appid = store.state.app.appid
  return request({
    url: `/api/CollaboratorsList?appid=${appid}`,
    method: 'get'
  })
}

/**
 * 删除应用的一个协作者
 * @param {string} collaborator_id
 */
export function removeCollaborator(collaborator_id) {
  const appid = store.state.app.appid
  return request({
    url: `/sys-api/apps/${appid}/collaborators/${collaborator_id}`,
    method: 'delete'
  })
}

/**
 * 请求我的应用
 * @returns 返回我的应用列表
 */
export function getMyApplications() {
  return request({
    url: '/api/ApplicationMy',
    method: 'get'
  })
}

/**
 * Get avaliable specs
 * @returns
 */
export function getSpecs() {
  return request({
    url: '/api/ApplicationSpecs',
    method: 'get'
  })
}

/**
 * 根据 appid 获取应用
 * @param {string} appid
 * @returns { Application } 返回应用数据
 */
export async function getApplicationByAppid(appid) {
  const res = await request({
    url: `/api/AppsInfo?appid=${appid}`,
    method: 'get'
  })

  return res
}

/**
 * 创建应用
 * @returns
 */
export async function createApplication({ name }) {
  const res = await request({
    url: '/api/ApplicationCreate',
    method: 'post',
    data: {
      name
    }
  })
  return res
}

/**
 * 编辑应用
 * @returns
 */
export async function updateApplication(appid, { name }) {
  const res = await request({
    url: `/api/ApplicationUpdate?appid=${appid}`,
    method: 'post',
    data: {
      name
    }
  })
  return res
}

/**
 * 删除（释放）应用
 * @param {*} appid
 * @returns
 */
export async function removeApplication(appid) {
  const res = await request({
    url: `/api/ApplicationRemove?appid=${appid}`,
    method: 'delete'
  })
  return res
}

/**
 * 添加协作成员
 * @param {member_id, roles}
 * @returns
 */
export async function inviteCollaborator(member_id, roles) {
  const appid = store.state.app.appid
  const res = await request({
    url: `/sys-api/apps/${appid}/invite`,
    method: 'post',
    data: {
      member_id,
      roles
    }
  })
  return res
}

/**
 * 获取所有应用角色
 * @param {string} username
 */
export function getAllApplicationRoles() {
  return request({
    url: '/api/CollaboratorsRoles',
    method: 'get'
  })
}

/**
 * 根据用户名搜索用户
 * @param {string} username
 */
export function searchUserByUsername(username) {
  return request({
    url: '/sys-api/apps/collaborators/search',
    method: 'post',
    data: {
      username
    }
  })
}

/**
 * 删除应用服务
 * @param {*} appid
 * @returns
 */
export async function removeApplicationService(appid) {
  const res = await request({
    url: `/sys-api/apps/${appid}/service/remove`,
    method: 'post'
  })
  return res
}

/**
 * 操作应用服务
 * @param {*} appid
 * @returns
 */
export async function opApplicationInstance(appid, op) {
  const res = await request({
    url: `/api/ApplicationInstanceOp?appid=${appid}`,
    method: 'post',
    data: {
      'op': op
    }
  })
  return res
}

/**
 * Open app console
 * @param {*} app
 */
export async function openAppConsole(app) {
  setCurrentAppid(app.appid)
  const back_url = encodeURIComponent(window.location.href)
  let app_console_url = `/#/app/${app.appid}/dashboard/index?$back_url=${back_url}`
  window.open(app_console_url, '_self')
}