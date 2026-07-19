import axios from 'axios'
const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
export const http = axios.create({ baseURL, timeout: 15000 })
http.interceptors.response.use(r => r, e => Promise.reject(e))
export const wsBase = import.meta.env.VITE_WS_BASE_URL || (location.origin.startsWith('https') ? location.origin.replace('https', 'wss') : location.origin.replace('http', 'ws'))