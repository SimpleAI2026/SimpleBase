/**
 * 首页「一句话建应用」模板（planv6.6 home-starter-prompts §2.1 / §4）。
 *
 * 只登记模板元数据与 i18n key，文案在 `locales/zh-CN.ts` + `en-US.ts` 的
 * `pages.home.starters.<id>.{title,desc,prompt}`。
 *
 * B1 只上线「数据通道为只读快照」的三个模板；需要匿名写入的 todo / booking / survey
 * 依赖 B3（云函数 access=public|enduser，需安全评审），评审前不展示。服务端白名单同口径
 * （`cloudagent.IsStarterTemplate`），这里不是安全边界。
 */
export type StarterOutput = 'table' | 'function' | 'cron' | 'site'

export interface StarterTemplate {
  /** 与后端 `starter_template` 白名单一致。 */
  id: string
  /** lucide 图标名（组件内映射为具体图标，避免常量层依赖 UI 库）。 */
  icon: 'dashboard' | 'portfolio' | 'notice'
  titleKey: string
  descKey: string
  /** 发送给助手的完整种子提示词。 */
  promptKey: string
  /** 卡片徽标：预计产出。 */
  outputs: StarterOutput[]
}

function tpl(id: string, icon: StarterTemplate['icon'], outputs: StarterOutput[]): StarterTemplate {
  const base = `pages.home.starters.${id}`
  return { id, icon, titleKey: `${base}.title`, descKey: `${base}.desc`, promptKey: `${base}.prompt`, outputs }
}

export const STARTER_TEMPLATES: StarterTemplate[] = [
  tpl('dashboard', 'dashboard', ['table', 'cron', 'site']),
  tpl('portfolio', 'portfolio', ['table', 'site']),
  tpl('notice', 'notice', ['table', 'cron', 'site'])
]

export function findStarterTemplate(id: string | null | undefined): StarterTemplate | undefined {
  const key = (id || '').trim()
  return STARTER_TEMPLATES.find((t) => t.id === key)
}
