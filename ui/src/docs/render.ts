import { marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'

export interface DocHeading { id: string; text: string; level: number }
export interface DocRender { html: string; toc: DocHeading[] }

function escapeHTML(value: string): string {
  return value.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] || c)
}

function slugify(text: string): string {
  return text.toLowerCase().normalize('NFKC').replace(/[^\p{L}\p{N}\s-]/gu, '').trim().replace(/\s+/g, '-') || 'section'
}

marked.setOptions({
  gfm: true,
  breaks: false
})

export function rewriteDocHref(href: string, moduleId: string): string {
  const url = href || ''
  if (!url || /^(?:https?:\/\/|mailto:|#)/i.test(url) || url.startsWith('/docs/')) return url
  const [path, hash] = url.split('#', 2)
  const parts = [moduleId]
  for (const part of path.replace(/^\.\//, '').split('/')) {
    if (part === '..') parts.pop()
    else if (part && part !== '.') parts.push(part)
  }
  if (!parts.length || parts[0] !== moduleId && path.startsWith('../../')) {
    const safe = path.replace(/^\.\.\/\.\.\//, '')
    return `https://github.com/linkxzhou/SimpleBase/blob/main/${safe}${hash ? `#${hash}` : ''}`
  }
  const slug = parts.pop()?.replace(/\.md$/i, '') || 'index'
  const mod = parts.join('/')
  return `/docs/${mod}${slug === 'index' || slug === 'README' ? '' : `/${slug}`}${hash ? `#${hash}` : ''}`
}

/** 将 Markdown 渲染为安全 HTML 与目录；同页重复标题生成稳定的递增锚点。 */
export function renderMarkdown(md: string, moduleId: string): DocRender {
  const toc: DocHeading[] = []
  try {
    const renderer = new marked.Renderer()
    const originLink = renderer.link.bind(renderer)
    const originTable = renderer.table.bind(renderer)
    const originQuote = renderer.blockquote.bind(renderer)
    const used = new Map<string, number>()

    renderer.heading = (token: Tokens.Heading) => {
      const match = token.text.match(/\s*\{#([^}]+)\}\s*$/)
      const text = match ? token.text.slice(0, match.index).trim() : token.text
      const plain = text.replace(/<[^>]*>|[`*_~\[\]]/g, '').trim()
      const base = match?.[1] || slugify(plain)
      const count = (used.get(base) || 0) + 1
      used.set(base, count)
      const id = count === 1 ? base : `${base}-${count}`
      if (token.depth === 2 || token.depth === 3) toc.push({ id, text: plain, level: token.depth })
      const label = String(marked.parseInline(text, { async: false }))
      const anchor = token.depth >= 2 && token.depth <= 4
        ? `<a class="docs-anchor" href="#${escapeHTML(id)}" aria-label="链接到 ${escapeHTML(plain)}">#</a>` : ''
      return `<h${token.depth} id="${escapeHTML(id)}">${label}${anchor}</h${token.depth}>\n`
    }
    renderer.code = (token: Tokens.Code) => {
      const lang = token.lang?.trim().split(/\s+/)[0] || 'text'
      return `<div class="docs-code" data-lang="${escapeHTML(lang)}"><div class="docs-code-bar"><span>${escapeHTML(lang)}</span><button type="button" data-copy aria-label="复制代码">复制</button></div><pre><code>${escapeHTML(token.text)}</code></pre></div>\n`
    }
    renderer.table = (token: Tokens.Table) => `<div class="docs-table-scroll">${originTable(token)}</div>`
    renderer.blockquote = (token: Tokens.Blockquote) => {
      const kind = token.text.match(/^\*\*(Note|Tip|Warning|注意|提示)\*\*/i)?.[1]?.toLowerCase()
      return originQuote(token).replace('<blockquote', `<blockquote${kind ? ` data-callout="${kind}"` : ''}`)
    }
    renderer.link = (token: Tokens.Link) => {
      const url = rewriteDocHref(token.href || '', moduleId)
      const html = String(originLink({ ...token, href: url }) ?? '')
      return /^https?:\/\//i.test(url) ? html.replace('<a ', '<a target="_blank" rel="noopener noreferrer" ') : html
    }

    const dirty = marked.parse(md, { renderer, async: false }) as string
    return { html: DOMPurify.sanitize(dirty, { USE_PROFILES: { html: true }, ADD_ATTR: ['target', 'data-copy', 'data-lang', 'data-callout'] }), toc }
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    return { html: `<p>文档渲染失败：${escapeHTML(msg)}</p>`, toc: [] }
  }
}
