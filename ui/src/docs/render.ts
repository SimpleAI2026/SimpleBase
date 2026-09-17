import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({
  gfm: true,
  breaks: false
})

/**
 * Render markdown to sanitized HTML.
 * Relative *.md links become /docs/:module/:slug (same module unless ../other/).
 */
export function renderMarkdown(md: string, moduleId: string): string {
  const renderer = new marked.Renderer()
  const originLink = renderer.link.bind(renderer)

  renderer.link = ({ href, title, text }) => {
    let url = href || ''
    if (url && !/^https?:\/\//i.test(url) && !url.startsWith('#') && !url.startsWith('mailto:')) {
      if (url.startsWith('/docs/')) {
        // already app path
      } else {
        // strip leading ./
        let rel = url.replace(/^\.\//, '')
        let mod = moduleId
        // handle ../module/file.md lightly
        const up = rel.match(/^\.\.\/([^/]+)\/(.+)$/)
        if (up) {
          mod = up[1]
          rel = up[2]
        }
        rel = rel.replace(/\.md$/i, '')
        if (rel === 'index' || rel === 'README') {
          url = `/docs/${mod}`
        } else {
          url = `/docs/${mod}/${rel}`
        }
      }
    }
    const html = originLink({ href: url, title, text, type: 'link', raw: '' })
    if (/^https?:\/\//i.test(url)) {
      return html.replace('<a ', '<a target="_blank" rel="noopener noreferrer" ')
    }
    return html
  }

  const dirty = marked.parse(md, { renderer }) as string
  return DOMPurify.sanitize(dirty, {
    USE_PROFILES: { html: true },
    ADD_ATTR: ['target']
  })
}
