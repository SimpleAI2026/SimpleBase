import store from '@/store'

export function openSystemClient() {
  const href = getSystemClientUrl() || '/'
  window.open(href, '_self')
}

export function setSystemClientUrl(back_url) {
  localStorage.setItem('system_client_url', back_url)
}

export function getSystemClientUrl() {
  return localStorage.getItem('system_client_url')
}

export function setCurrentAppid(appid) {
  localStorage.setItem('current_appid', appid)
}

export function getCurrentAppid() {
  return localStorage.getItem('current_appid')
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