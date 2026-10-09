import type { Component } from 'vue'
import {
  BotIcon,
  BoxIcon,
  BracesIcon,
  CloudUploadIcon,
  CodeIcon,
  DatabaseIcon,
  FileTextIcon,
  LayoutDashboardIcon,
  TimerIcon,
  UsersIcon
} from '@lucide/vue'

/**
 * 控制台侧栏路由名 → 图标。
 * 导航和云助手共用这一份定义，页面不要再单独引入一套同类图标。
 */
export const sidebarIcons: Record<string, Component> = {
  dashboard: LayoutDashboardIcon,
  databases: DatabaseIcon,
  'key-value': BracesIcon,
  s3: CloudUploadIcon,
  gofunctions: CodeIcon,
  'cron-jobs': TimerIcon,
  sandboxes: BoxIcon,
  agents: BotIcon,
  logs: FileTextIcon,
  users: UsersIcon
}

/**
 * Agent module / skill id → 侧栏路由名。
 * general、project 以及没有对应导航项的模块走默认助手图标（agents）。
 */
const moduleNavKey: Record<string, string> = {
  database: 'databases',
  kv: 'key-value',
  s3: 's3',
  gofunction: 'gofunctions',
  cron: 'cron-jobs',
  sandbox: 'sandboxes',
  logs: 'logs',
  users: 'users'
}

/** 返回与侧栏同一图标的路由名；未知模块为 agents。 */
export function navKeyForAgentModule(module?: string): string {
  const key = (module || '').trim().toLowerCase()
  return moduleNavKey[key] || 'agents'
}

/** 云助手 Agent 标识与对话头像使用的图标，与侧栏是同一个组件。 */
export function iconForAgentModule(module?: string): Component {
  return sidebarIcons[navKeyForAgentModule(module)]
}
