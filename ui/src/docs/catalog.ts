import metaJson from '../../../docs/_meta.json'

export interface DocPage {
  moduleId: string
  slug: string
  title: string
  order: number
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
const rawModules = import.meta.glob('../../../docs/**/*.md', {
  query: '?raw',
  import: 'default',
  eager: true
}) as Record<string, string>

export const loadedMarkdownCount = Object.keys(rawModules).length

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
  const m = body.match(/^#\s+(.+)$/m)
  return m?.[1]?.trim()
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
    const page: DocPage = { moduleId, slug, title, order, filePath: rel }
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
  return buildCatalogFrom(rawModules, meta)
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

/** Markdown body without frontmatter, or null if the glob did not include the file. */
export function loadMarkdownRaw(filePath: string): string | null {
  for (const [key, raw] of Object.entries(rawModules)) {
    const rel = docsRelPath(key)
    if (rel === filePath) {
      return parseFrontmatter(raw).body
    }
  }
  return null
}

export function githubBlobUrl(filePath: string): string {
  return `https://github.com/linkxzhou/SimpleBase/blob/main/docs/${filePath}`
}
