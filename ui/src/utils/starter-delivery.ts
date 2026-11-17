/**
 * 交付块协议（planv6.6 home-starter-prompts §2.8）。
 *
 * 助手在 verify 阶段最后输出 `<!-- sb:delivery {...} -->`（HTML 注释，Markdown 不渲染），
 * 前端解析为交付卡片。**前端只信结构不信 URL**：
 * - `site_url` 必须与 `GET /s3/site` 的 baseUrl 同源，否则丢弃、改用后端返回值；
 * - 函数地址由前端按 `location.origin + /go/<pid>/<库>/<函数>` 重新拼接，不用助手给的 url。
 * 字段缺失逐项隐藏；整体缺失返回 null，由调用方回退为普通消息。
 */
export interface StarterDeliveryFunction {
  name: string
  /** 助手给的原始 url 只用于取出「库名 / 导出名」，不会被直接展示。 */
  url: string
}

export interface StarterDeliveryCron {
  name: string
  expr: string
}

export interface StarterDelivery {
  siteUrl: string
  functions: StarterDeliveryFunction[]
  tables: string[]
  collections: string[]
  cron: StarterDeliveryCron[]
  next: string[]
}

const BLOCK_RE = /<!--\s*sb:delivery\s+([\s\S]*?)\s*-->/
/** 单个交付块的上限，防止异常大的注释拖慢渲染。 */
const MAX_BLOCK_CHARS = 16 * 1024
const MAX_ITEMS = 50

function strList(v: unknown): string[] {
  if (!Array.isArray(v)) return []
  return v
    .filter((x): x is string => typeof x === 'string')
    .map((x) => x.trim())
    .filter((x) => x !== '')
    .slice(0, MAX_ITEMS)
}

/** 从助手最终消息里取出交付块；缺失或损坏返回 null。 */
export function parseDelivery(content: string): StarterDelivery | null {
  if (!content || content.length > MAX_BLOCK_CHARS * 8) return null
  const m = BLOCK_RE.exec(content)
  if (!m || m[1].length > MAX_BLOCK_CHARS) return null
  let raw: unknown
  try {
    raw = JSON.parse(m[1])
  } catch {
    return null
  }
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null
  const o = raw as Record<string, unknown>

  const functions: StarterDeliveryFunction[] = []
  if (Array.isArray(o.functions)) {
    for (const f of o.functions.slice(0, MAX_ITEMS)) {
      if (!f || typeof f !== 'object') continue
      const fo = f as Record<string, unknown>
      const name = typeof fo.name === 'string' ? fo.name.trim() : ''
      if (!name) continue
      functions.push({ name, url: typeof fo.url === 'string' ? fo.url : '' })
    }
  }
  const cron: StarterDeliveryCron[] = []
  if (Array.isArray(o.cron)) {
    for (const c of o.cron.slice(0, MAX_ITEMS)) {
      if (!c || typeof c !== 'object') continue
      const co = c as Record<string, unknown>
      const name = typeof co.name === 'string' ? co.name.trim() : ''
      if (!name) continue
      cron.push({ name, expr: typeof co.expr === 'string' ? co.expr.trim() : '' })
    }
  }
  const delivery: StarterDelivery = {
    siteUrl: typeof o.site_url === 'string' ? o.site_url.trim() : '',
    functions,
    tables: strList(o.tables),
    collections: strList(o.collections),
    cron,
    next: strList(o.next)
  }
  return delivery
}

/** 去掉交付块注释，剩余正文作为普通 Markdown 展示。 */
export function stripDelivery(content: string): string {
  return content.replace(BLOCK_RE, '').trimEnd()
}

/**
 * 校验并规整站点地址：只接受 http(s)，且必须与后端给的 baseUrl 同源。
 * 后端没给 baseUrl（站点未启用 / 不可发布）时一律丢弃。返回要展示的完整 URL（落在 `/web/`）。
 */
export function resolveSiteUrl(claimed: string, backendBaseUrl: string | undefined): string {
  if (!backendBaseUrl) return ''
  let base: URL
  try {
    base = new URL(backendBaseUrl)
  } catch {
    return ''
  }
  if (base.protocol !== 'http:' && base.protocol !== 'https:') return ''
  if (claimed) {
    try {
      const c = new URL(claimed)
      if (c.origin !== base.origin) {
        // 助手给的地址与后端不同源：丢弃，改用后端值。
        return `${base.origin}/web/`
      }
    } catch {
      /* 非法 URL 也回退为后端值 */
    }
  }
  return `${base.origin}/web/`
}

/**
 * 云函数调用地址：由前端拼，不信助手给的 url。助手 url 形如 `/go/<pid>/<库名>/<导出>`，
 * 只取库名段；缺失时回退到 `main`（与计划 §2.8 示例一致）。
 */
export function functionCallUrl(origin: string, projectId: string, fn: StarterDeliveryFunction): string {
  const m = /^\/go\/[^/]+\/([^/?#]+)\//.exec(fn.url)
  const lib = m ? m[1] : 'main'
  const seg = (s: string) => encodeURIComponent(s)
  return `${origin.replace(/\/+$/, '')}/go/${seg(projectId)}/${seg(lib)}/${seg(fn.name)}`
}
