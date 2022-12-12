import request from '@/utils/request'

/**
 * 账户登录
 * @param {username: string, password: string} data
 * @returns
 */
export function login(data) {
  return request({
    url: '/api/AccountLogin',
    method: 'post',
    data
  })
}

/**
 * 获取帐户信息
 * @param {string} token
 * @returns
 */
export function getUserProfile() {
  return request({
    url: '/api/AccountProfile',
    method: 'get'
  })
}

/**
 * 注册帐户
 */
export function signup(data) {
  return request({
    url: '/api/AccountSignup',
    method: 'post',
    data
  })
}
