import { login, getUserProfile } from '@/api/user'
import { getToken, setToken, removeToken } from '@/utils/auth'

import { Message } from 'element-ui'

const state = {
  token: getToken(),
  userProfile: null
}

const mutations = {
  SET_TOKEN: (state, token) => {
    state.token = token
  },
  SET_USER_PROFILE: (state, userProfile) => {
    state.userProfile = userProfile
  }
}

const actions = {
  // user login
  async login({ commit }, { username, password }) {
    const res = await login({ username: username.trim(), password: password })
    const { data } = res
    if (res.error) {
      Message.error(res.error)
      return
    }
    commit('SET_TOKEN', data.access_token, data.expire)
    setToken(data.access_token, data.expire)
  },

  // get user info
  async getInfo({ commit, dispatch }) {
    const res = await getUserProfile()
    const { data } = res
    if (!data) {
      return dispatch('/user/logout')
    }
    const account = data
    commit('SET_USER_PROFILE', account)
    return data
  },

  // user logout
  logout({ commit, state, dispatch }) {
    return new Promise((resolve, reject) => {
      removeToken()

      resolve()
    })
  },

  // remove token
  resetToken({ commit }) {
    return new Promise(resolve => {
      commit('SET_TOKEN', '')
      commit('SET_USER_PROFILE', null)
      commit('SET_USER_PROFILE', null)
      removeToken()
      resolve()
    })
  }
}

export default {
  namespaced: true,
  state,
  mutations,
  actions
}
