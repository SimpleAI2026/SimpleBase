import { marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({
  gfm: true,
  breaks: false
})

function rewriteDocHref(href: string, moduleId: string): string {
  let url = href || ''
  if (!url || /^https?:\/\//i.test(url) || url.startsWith('#') || url.startsWith('mailto:')) {
    return url
  }
  if (url.startsWith('/docs/')) return url
  let rel = url.replace(/^\.\//, '')
  let mod = moduleId
  const up = rel.match(/^\.\.\/([^/]+)\/(.+)$/)
  if (up) {
    mod = up[1]
    rel = up[2]
  }
  rel = rel.replace(/\.md$/i, '')
  if (rel === 'index' || rel === 'README') return `/docs/${mod}`
  return `/docs/${mod}/${rel}`
}

/**
 * Render markdown to sanitized HTML.
 * Relative *.md links become /docs/:module/:slug (same module unless ../other/).
 * Never throws — callers must always get a visible string.
 */
export function renderMarkdown(md: string, moduleId: string): string {
  try {
    const renderer = new marked.Renderer()
    const originLink = renderer.link.bind(renderer)

    renderer.link = (token: Tokens.Link) => {
      const url = rewriteDocHref(token.href || '', moduleId)
      const html = String(originLink({ ...token, href: url }) ?? '')
      if (/^https?:\/\//i.test(url)) {
        return html.replace('<a ', '<a target="_blank" rel="noopener noreferrer" ')
      }
      return html
    }

    const dirty = marked.parse(md, { renderer, async: false }) as string
    return DOMPurify.sanitize(dirty, {
      USE_PROFILES: { html: true },
      ADD_ATTR: ['target']
    })
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    return `<p>文档渲染失败：${msg.replace(/[<>&]/g, (c) => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;' }[c] as string))}</p>`
  }
}
