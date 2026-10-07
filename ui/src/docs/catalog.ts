import metaJson from '@docs/_meta.json'
import { marked } from 'marked'
import { renderMarkdown } from './render'

export interface DocPage {
  moduleId: string
  slug: string
  title: string
  order: number
  group?: string
  description?: string
  /** path relative to docs/, e.g. ops/deployment.md */
  filePath: string
}

export interface DocModule {
  id: string
  title: string
  order: number
  pages: DocPage[]
}

type MetaFile = {
  modules?: { id: string; title: string; order?: number }[]
}

const meta = metaJson as MetaFile

/**
 * Vite `import.meta.glob` only accepts relative (`./` `../`) or project-root
 * (`/`) patterns — aliases like `@docs/**` are silently empty.
 * Path is from this file (`ui/src/docs/`) up to repo `docs/`.
 */
const rawModules = import.meta.glob(['../../../docs/**/*.md', '!../../../docs/database/ducklake.md'], {
  query: '?raw', import: 'default', eager: true
}) as Record<string, string>
const largeModules = import.meta.glob('../../../docs/database/ducklake.md', {
  query: '?raw', import: 'default'
}) as Record<string, () => Promise<string>>
const largeMeta = '---\ntitle: DuckLake 上游参考\norder: 99\n---\n# DuckLake 上游参考'
const catalogEntries: Record<string, string> = { ...rawModules,
  ...Object.fromEntries(Object.keys(largeModules).map((key) => [key, largeMeta])) }
const loadedBodies = new Map<string, string>()

export const loadedMarkdownCount = Object.keys(catalogEntries).length

export function parseFrontmatter(raw: string): { data: Record<string, string | number>; body: string } {
  if (!raw.startsWith('---')) return { data: {}, body: raw }
  const end = raw.indexOf('\n---', 3)
  if (end < 0) return { data: {}, body: raw }
  const fm = raw.slice(3, end).trim()
  const body = raw.slice(end + 4).replace(/^\n/, '')
  const data: Record<string, string | number> = {}
  for (const line of fm.split('\n')) {
    const m = line.match(/^([A-Za-z0-9_-]+):\s*(.*)$/)
    if (!m) continue
    const key = m[1]
    let val: string | number = m[2].trim()
    if (/^\d+$/.test(val)) val = Number(val)
    else if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1)
    }
    data[key] = val
  }
  return { data, body }
}

export function firstH1(body: string): string | undefined {
  const tokens = marked.lexer(body)
  const heading = tokens.find((token) => token.type === 'heading' && token.depth === 1)
  return heading?.type === 'heading' ? heading.text.replace(/\s*\{#[^}]+\}\s*$/, '').trim() : undefined
}

export function toPosix(p: string): string {
  return p.replace(/\\/g, '/')
}

/** Normalize glob key → docs-relative path like `ops/deployment.md`. */
export function docsRelPath(globKey: string): string | null {
  const key = toPosix(globKey)
  const idx = key.lastIndexOf('/docs/')
  if (idx >= 0) return key.slice(idx + '/docs/'.length)
  const at = key.indexOf('@docs/')
  if (at >= 0) return key.slice(at + '@docs/'.length)
  return null
}

export function buildCatalogFrom(
  entries: Record<string, string>,
  metaFile: MetaFile = meta
): DocModule[] {
  const pagesByModule = new Map<string, DocPage[]>()

  for (const [key, raw] of Object.entries(entries)) {
    const rel = docsRelPath(key)
    if (!rel || rel.includes('/_')) continue
    const parts = rel.split('/')
    if (parts.length < 2) continue // ignore flat leftovers
    const moduleId = parts[0]
    const fileName = parts[parts.length - 1]
    if (!fileName.endsWith('.md')) continue
    const base = fileName.replace(/\.md$/, '')
    const slug = base === 'index' || base === 'README' ? 'index' : base
    const { data, body } = parseFrontmatter(raw)
    const title =
      (typeof data.title === 'string' && data.title) ||
      firstH1(body) ||
      slug
    const order = typeof data.order === 'number' ? data.order : slug === 'index' ? 0 : 100
    const page: DocPage = { moduleId, slug, title, order, filePath: rel,
      group: typeof data.group === 'string' ? data.group : undefined,
      description: typeof data.description === 'string' ? data.description : undefined }
    const list = pagesByModule.get(moduleId) || []
    list.push(page)
    pagesByModule.set(moduleId, list)
  }

  const metaMods = metaFile.modules || []
  const modules: DocModule[] = []

  for (const m of metaMods) {
    const pages = (pagesByModule.get(m.id) || []).slice().sort((a, b) => a.order - b.order || a.slug.localeCompare(b.slug))
    pagesByModule.delete(m.id)
    modules.push({
      id: m.id,
      title: m.title,
      order: m.order ?? 100,
      pages
    })
  }

  // leftover dirs not in meta
  for (const [id, pages] of pagesByModule) {
    modules.push({
      id,
      title: id,
      order: 1000,
      pages: pages.slice().sort((a, b) => a.order - b.order || a.slug.localeCompare(b.slug))
    })
  }

  return modules.filter((m) => m.pages.length > 0).sort((a, b) => a.order - b.order)
}

function buildCatalog(): DocModule[] {
  return buildCatalogFrom(catalogEntries, meta)
}

export const docCatalog: DocModule[] = buildCatalog()

export function getModule(id: string): DocModule | undefined {
  return docCatalog.find((m) => m.id === id)
}

export function getPage(moduleId: string, slug: string): DocPage | undefined {
  return getModule(moduleId)?.pages.find((p) => p.slug === slug)
}

export function defaultModuleId(): string {
  return docCatalog[0]?.id || 'getting-started'
}

export function defaultSlug(moduleId: string): string {
  const mod = getModule(moduleId)
  if (!mod?.pages.length) return 'index'
  const idx = mod.pages.find((p) => p.slug === 'index')
  return idx?.slug || mod.pages[0].slug
}

/** 已同步打包的小文件正文（不包含 DuckLake 上游镜像）。 */
export function loadMarkdownRaw(filePath: string): string | null {
  if (loadedBodies.has(filePath)) return loadedBodies.get(filePath) || null
  for (const [key, raw] of Object.entries(rawModules)) {
    if (docsRelPath(key) === filePath) return parseFrontmatter(raw).body
  }
  return null
}

/** 巨型参考文档只有被打开或首次搜索时才请求，失败允许再次尝试。 */
export async function loadMarkdown(filePath: string): Promise<string | null> {
  const cached = loadMarkdownRaw(filePath)
  if (cached !== null) return cached
  const loader = Object.entries(largeModules).find(([key]) => docsRelPath(key) === filePath)?.[1]
  if (!loader) return null
  const body = parseFrontmatter(await loader()).body
  loadedBodies.set(filePath, body)
  return body
}

export function isLargeDoc(filePath: string): boolean {
  return Object.keys(largeModules).some((key) => docsRelPath(key) === filePath)
}

export interface DocSearchResult { page: DocPage; heading?: string; hash?: string; snippet: string }

let searchIndex: Promise<DocSearchResult[]> | null = null
export async function searchDocs(query: string): Promise<DocSearchResult[]> {
  const term = query.trim().toLocaleLowerCase()
  if (!term) return []
  searchIndex ||= Promise.all(docCatalog.flatMap((module) => module.pages).map(async (page) => {
    const body = await loadMarkdown(page.filePath) || ''
    const headings = renderMarkdown(body, page.moduleId).toc
    return { page, body: body.replace(/```[\s\S]*?```/g, '').replace(/<[^>]+>/g, ' '), headings }
  })).then((entries) => entries.flatMap(({ page, body, headings }) => [
    { page, snippet: page.description || page.title },
    ...headings.map(({ text, id }) => ({ page, heading: text, hash: id, snippet: text })),
    { page, snippet: body.replace(/\s+/g, ' ') }
  ]))
  try {
    const entries = await searchIndex
    return entries.filter((item) => `${item.page.title} ${item.heading || ''} ${item.snippet}`.toLocaleLowerCase().includes(term))
      .sort((a, b) => Number(b.page.title.toLocaleLowerCase().includes(term)) - Number(a.page.title.toLocaleLowerCase().includes(term)) || Number(!!b.heading) - Number(!!a.heading))
      .slice(0, 20).map((item) => ({ ...item, snippet: item.snippet.length > 140 ? `${item.snippet.slice(Math.max(0, item.snippet.toLocaleLowerCase().indexOf(term) - 45), item.snippet.toLocaleLowerCase().indexOf(term) + 90)}…` : item.snippet }))
  } catch {
    searchIndex = null
    return []
  }
}

export function githubBlobUrl(filePath: string): string {
  return `https://github.com/linkxzhou/SimpleBase/blob/main/docs/${filePath}`
}
