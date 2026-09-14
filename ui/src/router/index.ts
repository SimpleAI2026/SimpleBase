import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import Dashboard from '../pages/Dashboard.vue'
import DataManager from '../pages/DataManager.vue'
import S3Manager from '../pages/S3Manager.vue'
import FaaSManager from '../pages/FaaSManager.vue'
import LlmManager from '../pages/LlmManager.vue'
import Logs from '../pages/Logs.vue'

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'dashboard', component: Dashboard },
  { path: '/data', name: 'data', component: DataManager },
  { path: '/s3', name: 's3', component: S3Manager },
  { path: '/faas', name: 'faas', component: FaaSManager },
  { path: '/llm', name: 'llm', component: LlmManager },
  { path: '/logs', name: 'logs', component: Logs }
]
export default createRouter({ history: createWebHistory(), routes })