import store from '@/store'
import request from '@/utils/request'

/**
 * 获取应用的协作者列表
 * @param {string} username
 */
export function getCollaborators() {
  const appid = store.state.app.appid
  return request({
    url: `/sys-api/apps/${appid}/collaborators`,
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
 * 更新协作者密码(TBD)
 * @param {string} accountId
 * @param {string} password
 */
export function resetAccountPassword(accountId, password) {
  return request({
    url: '/sys-api/account/resetPassword',
    method: 'post',
    data: {
      accountId,
      password
    }
  })
}

/**
 * 请求我的应用
 * @returns 返回我的应用列表
 */
export function getMyApplications() {
  return request({
    url: '/api?fn=ApplicationMy',
    method: 'get'
  })
}

/**
 * Get avaliable specs
 * @returns
 */
export function getSpecs() {
  return request({
    url: '/api?fn=ApplicationSpecs',
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
    url: `/sys-api/apps/${appid}`,
    method: 'get'
  })

  return res
}

/**
 * 创建应用
 * @param param0
 * @returns
 */
export async function createApplication({ name }) {
  const res = await request({
    url: `/sys-api/apps/create`,
    method: 'post',
    data: {
      name
    }
  })
  return res
}

/**
 * 编辑应用
 * @param param0
 * @returns
 */
export async function updateApplication(appid, { name }) {
  const res = await request({
    url: `/sys-api/apps/${appid}`,
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
    url: `/sys-api/apps/${appid}`,
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
    url: '/sys-api/apps/collaborators/roles',
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
 * 重启应用服务
 * @param {*} appid
 * @returns
 */
export async function restartApplicationInstance(appid) {
  const res = await request({
    url: `/sys-api/apps/${appid}/instance/restart`,
    method: 'post'
  })
  return res
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
 * 获取应用的依赖
 * @param {*} appid
 * @returns
 */
export async function getApplicationPackages(appid) {
  const res = await request({
    url: `/sys-api/apps/${appid}/packages`,
    method: 'get'
  })
  return res
}

/**
 * 添加应用的依赖
 * @param {*} appid
 * @returns
 */
export async function addApplicationPackage(appid, { name, version }) {
  const res = await request({
    url: `/sys-api/apps/${appid}/packages`,
    method: 'post',
    data: { name, version }
  })
  return res
}

/**
 * 更新应用的依赖
 * @param {*} appid
 * @returns
 */
export async function updateApplicationPackage(appid, { name, version }) {
  const res = await request({
    url: `/sys-api/apps/${appid}/packages`,
    method: 'put',
    data: { name, version }
  })
  return res
}

/**
 * 删除应用的依赖
 * @param {*} appid
 * @returns
 */
export async function removeApplicationPackage(appid, name) {
  const res = await request({
    url: `/sys-api/apps/${appid}/packages`,
    method: 'delete',
    data: { name }
  })
  return res
}

/**
 * 获取当前应用的访问地址
 * @param {*} appid default is current appid
 * @returns
 */
export function getAppAccessUrl() {
  const appid = store.state.app.appid
  const domain = store.state.app.app_deploy_host
  const schema = store.state.app.app_deploy_url_schema || 'http'
  const url = `${schema}://${appid}.${domain}`
  return url
}

/**
 * 启动应用服务
 * @param {*} appid
 * @returns
 */
export async function startApplicationInstance(appid) {
  const res = await request({
    url: `/sys-api/apps/${appid}/instance/start`,
    method: 'post'
  })
  return res
}

/**
 * 停止应用服务
 * @param {*} appid
 * @returns
 */
export async function stopApplicationInstance(appid) {
  const res = await request({
    url: `/sys-api/apps/${appid}/instance/stop`,
    method: 'post'
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
  console.log("app_console_url: ", app_console_url)
  window.open(app_console_url, '_self')
}

export function setCurrentAppid(appid) {
  localStorage.setItem("current_appid", appid)
}

export function getCurrentAppid() {
  return localStorage.getItem("current_appid")
}