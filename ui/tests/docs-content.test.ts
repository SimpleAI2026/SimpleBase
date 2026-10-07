import { describe, expect, it } from 'vitest'
import { marked, type Tokens } from 'marked'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { docCatalog, parseFrontmatter } from '@/docs/catalog'
import { renderMarkdown, rewriteDocHref } from '@/docs/render'

const root = resolve(import.meta.dirname, '../../docs')
const files = new Map(docCatalog.flatMap((module) => module.pages).map((page) => [page.filePath, readFileSync(resolve(root, page.filePath), 'utf8')]))
const pages = docCatalog.flatMap((module) => module.pages)
const paths = new Set(pages.map((p) => p.filePath))

function targetFor(href: string, moduleId: string): { file: string; anchor: string } | null {
  const normalized = rewriteDocHref(href, moduleId)
  if (!normalized.startsWith('/docs/')) return null
  const [route, anchor = ''] = normalized.split('#')
  const parts = route.slice('/docs/'.length).split('/')
  const file = `${parts[0]}/${parts.slice(1).join('/') || 'index'}.md`
  return { file, anchor: decodeURIComponent(anchor) }
}

function markdownLinks(body: string): string[] {
  const links: string[] = []
  const renderer = new marked.Renderer()
  renderer.link = (token: Tokens.Link) => { links.push(token.href || ''); return '' }
  marked.parse(body, { renderer, async: false })
  return links
}

describe('文档内容完整性', () => {
  it('每篇都有标题、排序与一级标题（上游镜像按源文档豁免）', () => {
    for (const page of pages) {
      const raw = files.get(page.filePath)
      expect(raw, page.filePath).toBeDefined()
      const { data, body } = parseFrontmatter(raw!)
      expect(data.title, page.filePath).toBeTruthy()
      expect(typeof data.order, page.filePath).toBe('number')
      if (page.filePath !== 'database/ducklake.md') {
        const headings = marked.lexer(body).filter((token) => token.type === 'heading' && token.depth === 1)
        expect(headings.length, page.filePath).toBe(1)
      }
    }
  })

  it('站内 Markdown 链接与目标锚点存在', () => {
    const errors: string[] = []
    const anchors = new Map<string, Set<string>>()
    for (const page of pages) {
      const { body } = parseFrontmatter(files.get(page.filePath) || '')
      const html = renderMarkdown(body, page.moduleId).html
      const ids = new Set([...html.matchAll(/<h[1-6][^>]* id="([^"]+)"/g)].map((match) => match[1]))
      anchors.set(page.filePath, ids)
    }
    for (const page of pages) {
      const body = parseFrontmatter(files.get(page.filePath) || '').body
      for (const href of markdownLinks(body)) {
        const target = href.startsWith('#') ? { file: page.filePath, anchor: decodeURIComponent(href.slice(1)) } : targetFor(href, page.moduleId)
        if (!target) continue
        if (!paths.has(target.file)) errors.push(`${page.filePath}: ${href} → ${target.file}`)
        else if (target.anchor && !anchors.get(target.file)?.has(target.anchor)) errors.push(`${page.filePath}: ${href} → #${target.anchor}`)
      }
    }
    expect(errors).toEqual([])
  })

  it('每个目录在模块清单中登记，且没有空模块', () => {
    const modules = new Set(docCatalog.map((item) => item.id))
    expect(docCatalog.every((item) => item.pages.length > 0)).toBe(true)
    for (const file of files.keys()) {
      if (!file.includes('/') || file.includes('/_')) continue
      expect(modules.has(file.split('/')[0]), file).toBe(true)
    }
  })

  it('代码围栏均标注语言', () => {
    const missing: string[] = []
    for (const [file, raw] of files) {
      if (file === 'database/ducklake.md') continue // 上游 HTML details 在 marked lexer 中会将有语言的围栏误判为无语言代码
      const body = parseFrontmatter(raw).body
      for (const token of marked.lexer(body)) {
        if (token.type === 'code' && !token.lang) missing.push(file)
      }
    }
    expect(missing).toEqual([])
  })
})
