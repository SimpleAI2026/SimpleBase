/**
 * 从 tokens.css 派生的 TS 常量：供 App.vue 的 antd theme token、
 * Dashboard 统计卡配色等 JS 侧使用，保证「单一变量源」。
 * 数值必须与 styles/tokens.css 中的定义保持一致（改一处改两处）。
 */

export const colors = {
  primary: '#d97757',
  primaryHover: '#c15e3c',
  success: '#3f8a5a',
  warning: '#c99a2c',
  danger: '#c0452f',
  info: '#4c7d9e',
  bgLayout: '#f5f4ef',
  text: '#1f1e1d',
  textSecondary: '#706f6a'
} as const

/** 语义色 12% 透明浅底（与 --sb-*-bg 变量一致），供统计卡图标底色复用 */
export const softBg = (hex: string, alpha = 0.12): string => {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

/** App.vue 的 antd ConfigProvider theme token（与 tokens.css 色板一致） */
export const antdThemeToken = {
  token: {
    colorPrimary: colors.primary,
    colorInfo: colors.primary,
    colorSuccess: colors.success,
    colorWarning: colors.warning,
    colorError: colors.danger,
    colorBgLayout: colors.bgLayout,
    colorText: colors.text,
    colorTextSecondary: colors.textSecondary,
    borderRadius: 10
  }
} as const


export const darkColors = {
  primary: '#d97757',
  primaryHover: '#e08b6d',
  success: '#5aa876',
  warning: '#d4b04a',
  danger: '#e06b57',
  info: '#6a9bb8',
  bgLayout: '#121110',
  text: '#f3f1ea',
  textSecondary: '#a8a59c'
} as const

export const antdDarkThemeToken = {
  token: {
    colorPrimary: darkColors.primary,
    colorInfo: darkColors.primary,
    colorSuccess: darkColors.success,
    colorWarning: darkColors.warning,
    colorError: darkColors.danger,
    colorBgLayout: darkColors.bgLayout,
    colorText: darkColors.text,
    colorTextSecondary: darkColors.textSecondary,
    borderRadius: 10
  }
} as const
