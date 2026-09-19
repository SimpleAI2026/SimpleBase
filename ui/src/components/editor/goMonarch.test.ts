import { describe, expect, it } from 'vitest'
import { goMonarchLanguage } from './goMonarch'

describe('goMonarchLanguage', () => {
  it('declares Go keywords, types, and tokenizer states', () => {
    expect(goMonarchLanguage.id).toBe('go')
    expect(goMonarchLanguage.keywords).toContain('func')
    expect(goMonarchLanguage.typeKeywords).toContain('string')
    expect(goMonarchLanguage.tokenizer.root.length).toBeGreaterThan(5)
    expect(goMonarchLanguage.tokenizer.string).toBeTruthy()
    expect(goMonarchLanguage.tokenizer.comment).toBeTruthy()
  })
})
