/**
 * 后端响应（wire）读取小工具：集中处理「字段缺失 / 类型不对 → 安全默认值」，
 * 取代散落各处的 `Array.isArray(x) ? x : []`。
 */

/** 后端 JSON 对象：字段类型未知，读取时必须显式转换。 */
export interface WireRecord {
  [key: string]: unknown
}

/**
 * 非数组一律得到 []。
 * - `listOf(v, map)`：逐项映射，入参类型由 map 决定（后端 JSON 无静态类型，信任点集中在 map）。
 * - `listOf<T>(v)`：元素类型由调用方断言；省略 T 时是 `never[]`，可直接 `.map(fn)`，
 *   与改造前 axios `r.data.x`（any）的使用方式一致，且不引入显式 any。
 */
export function listOf<T = never>(value: unknown): T[]
export function listOf<T, R>(value: unknown, map: (raw: T) => R): R[]
export function listOf<T, R>(value: unknown, map?: (raw: T) => R): T[] | R[] {
  const list = (Array.isArray(value) ? value : []) as T[]
  return map ? list.map((item) => map(item)) : list
}

/**
 * 逐字段读取：把「信任后端字段类型」收成一次显式的类型断言，转换函数里不再需要 `any`。
 * 语义与改造前的 `raw.x` / `raw.x ?? d` / `raw.x || undefined` / `raw.x || d` 一一对应，运行时行为不变。
 */

/** 原样读取（等价于 `raw.x`）；缺失时是 undefined，不做转换。 */
export function pick<T>(raw: WireRecord | undefined, key: string): T {
  return raw?.[key] as T
}

/** 缺失（null / undefined）才用默认值（等价于 `raw.x ?? d`）。 */
export function orDefault<T>(raw: WireRecord | undefined, key: string, fallback: T): T {
  return (raw?.[key] ?? fallback) as T
}

/** 假值（'' / 0 / false / null / undefined）一律当作「未设置」（等价于 `raw.x || undefined`）。 */
export function orUndefined<T = string>(raw: WireRecord | undefined, key: string): T | undefined {
  return (raw?.[key] || undefined) as T | undefined
}

/** 假值时用默认值（等价于 `raw.x || d`）。 */
export function orElse<T>(raw: WireRecord | undefined, key: string, fallback: T): T {
  return (raw?.[key] || fallback) as T
}

/** 嵌套对象：不是对象得到 undefined，便于继续用上面的读取器。 */
export function objectOf(value: unknown): WireRecord | undefined {
  return value && typeof value === 'object' ? (value as WireRecord) : undefined
}
