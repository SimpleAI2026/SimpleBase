import { describe, expect, it } from 'vitest'
import { cn } from './utils'

describe('cn', () => {
  it('merges tailwind classes and drops falsy values', () => {
    expect(cn('px-2', false && 'hidden', 'px-4')).toBe('px-4')
    expect(cn('text-sm', { 'font-bold': true, italic: false })).toBe('text-sm font-bold')
  })
})
