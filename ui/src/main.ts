import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import App from './App.vue'
import router from './router'
import { i18n } from './locales'
import { useSettingsStore } from './stores/settings'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
// i18n 必须在任何可能触发 http.ts 错误处理的调用之前安装：
// services / utils / composables 走 i18n.global.t，未安装时取不到消息。
app.use(i18n)
app.use(router)

// 把 localStorage 里的语言应用到 i18n 实例与 <html lang>，首屏即按用户选择渲染。
useSettingsStore(pinia).applyLocaleToI18n()

app.mount('#app')
