/** 与 internal/cloudagent/skills.go 的别名表一致。精确别名优先于同名助手。 */
const skillAliases: Record<string, string> = {
  '数据库': 'database',
  database: 'database',
  db: 'database',
  '键值': 'kv',
  kv: 'kv',
  '对象存储': 's3',
  s3: 's3',
  '对象': 's3',
  '云函数': 'gofunction',
  '函数': 'gofunction',
  gofunction: 'gofunction',
  '定时任务': 'cron',
  '定时': 'cron',
  cron: 'cron',
  '沙盒': 'sandbox',
  sandbox: 'sandbox',
  '日志': 'logs',
  logs: 'logs',
  '用户': 'users',
  users: 'users',
  '项目': 'project',
  project: 'project'
}

/** 把 @ token 解析成 skill id。精确助手名（如 Database）不是 skill。 */
export function resolveSkillAlias(token: string, agentNames: string[] = []): string {
  const name = token.trim().replace(/^@/, '')
  if (!name) return ''
  if (skillAliases[name]) return skillAliases[name]
  if (agentNames.includes(name)) return ''
  return skillAliases[name.toLowerCase()] || ''
}

export function skillsInText(text: string, agentNames: string[] = []): string[] {
  const ids: string[] = []
  const re = /@([^\s@]+)/g
  let match: RegExpExecArray | null
  while ((match = re.exec(text))) {
    const id = resolveSkillAlias(match[1], agentNames)
    if (id && !ids.includes(id)) ids.push(id)
  }
  return ids
}
