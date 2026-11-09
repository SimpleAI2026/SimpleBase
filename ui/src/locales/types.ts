/**
 * i18n 类型：语言编码与消息树。
 *
 * 对象形状一律用 interface（仓库统一规则）；消息树允许嵌套，
 * 叶子必须是 string —— 保证 zh-CN 与 en-US 同构校验时可递归比对。
 */

export type LocaleCode = 'zh-CN' | 'en-US'

export interface MessageTree {
  [key: string]: string | MessageTree
}
