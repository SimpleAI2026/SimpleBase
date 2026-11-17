/**
 * 生成「站点发布安全」的 8 位项目 ID（planv6.6 home-starter-prompts §2.2）。
 *
 * 后端项目 ID 校验是 `^[A-Za-z0-9-]{8}$`，但站点发布要求 DNS 安全
 * （`project_id_not_dns_safe`：不含大写、首尾不是 `-`）。一键流程只产出
 * `[a-z0-9]{8}`，天然满足两者。使用 Web Crypto，避免 Math.random 的可预测性。
 */
const ALPHABET = 'abcdefghijklmnopqrstuvwxyz0123456789'
const ID_LENGTH = 8

export function genDnsSafeId(random: (n: number) => Uint8Array = defaultRandom): string {
  const bytes = random(ID_LENGTH)
  let out = ''
  for (let i = 0; i < ID_LENGTH; i++) {
    // 256 % 36 !== 0，取模有极小偏差；项目 ID 不是密钥，可接受。
    out += ALPHABET[bytes[i] % ALPHABET.length]
  }
  return out
}

function defaultRandom(n: number): Uint8Array {
  const bytes = new Uint8Array(n)
  const c = (globalThis as { crypto?: { getRandomValues?: (a: Uint8Array) => Uint8Array } }).crypto
  if (c?.getRandomValues) {
    c.getRandomValues(bytes)
  } else {
    for (let i = 0; i < n; i++) bytes[i] = Math.floor(Math.random() * 256)
  }
  return bytes
}

/** 与后端 `config.ProjectIDPublishable` 同口径的本地预检。 */
export function isDnsSafeProjectId(id: string): boolean {
  return /^[a-z0-9]{8}$/.test(id)
}
