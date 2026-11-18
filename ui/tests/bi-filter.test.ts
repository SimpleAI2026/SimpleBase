import { describe, expect, it } from 'vitest'
import {
  RANGE_KEYS,
  binChip,
  bucketChip,
  bucketRange,
  chipFor,
  combine,
  pushHistory,
  quote,
  rangeExpr
} from '@/utils/bi-filter'

const NOW = new Date('2026-11-18T10:30:00Z')

describe('rangeExpr', () => {
  it('computes UTC RFC3339 lower bounds', () => {
    expect(rangeExpr('h1', 'created_at', NOW)).toBe('created_at>="2026-11-18T09:30:00.000Z"')
    expect(rangeExpr('h24', 'created_at', NOW)).toBe('created_at>="2026-11-17T10:30:00.000Z"')
    expect(rangeExpr('d7', 'placed', NOW)).toBe('placed>="2026-11-11T10:30:00.000Z"')
    expect(rangeExpr('d30', 'created_at', NOW)).toBe('created_at>="2026-10-19T10:30:00.000Z"')
  })
  it('all time adds no condition', () => {
    expect(rangeExpr('all', 'created_at', NOW)).toBe('')
  })
  it('offers the five ranges in order', () => {
    expect(RANGE_KEYS).toEqual(['h1', 'h24', 'd7', 'd30', 'all'])
  })
})

describe('combine', () => {
  it('drops empty parts and returns a single part untouched', () => {
    expect(combine([])).toBe('')
    expect(combine(['', '  ', undefined])).toBe('')
    expect(combine(['a=1'])).toBe('a=1')
    expect(combine(['  a=1  ', undefined])).toBe('a=1')
  })
  it('parenthesises each part so || cannot leak across parts', () => {
    expect(combine(['a=1 || b=2', 'c=3'])).toBe('(a=1 || b=2) && (c=3)')
    expect(combine(['a=1', '', 'b=2', 'c=3'])).toBe('(a=1) && (b=2) && (c=3)')
  })
})

describe('quote', () => {
  it('escapes quotes and backslashes so a value cannot break out of its literal', () => {
    expect(quote('open')).toBe('"open"')
    expect(quote('a"b')).toBe('"a\\"b"')
    expect(quote('a\\b')).toBe('"a\\\\b"')
    expect(quote('x" || 1=1 || y="')).toBe('"x\\" || 1=1 || y=\\""')
  })
})

describe('chipFor', () => {
  it('builds typed expressions', () => {
    expect(chipFor('status', 'paid', 'select')).toEqual({ id: 'status="paid"', label: 'status=paid', expr: 'status="paid"' })
    expect(chipFor('amount', '30', 'number')?.expr).toBe('amount=30')
    expect(chipFor('amount', 30, 'number')?.expr).toBe('amount=30')
    expect(chipFor('paid', true, 'bool')?.expr).toBe('paid=true')
    expect(chipFor('paid', 'false', 'bool')?.expr).toBe('paid=false')
  })
  it('falls back to a string literal when the value does not fit the type', () => {
    expect(chipFor('amount', 'abc', 'number')?.expr).toBe('amount="abc"')
    expect(chipFor('paid', 'maybe', 'bool')?.expr).toBe('paid="maybe"')
  })
  it('escapes hostile values', () => {
    expect(chipFor('title', 'x" || y="1', 'text')?.expr).toBe('title="x\\" || y=\\"1"')
  })
  it('rejects empty values and unsafe field names', () => {
    expect(chipFor('status', '', 'select')).toBeNull()
    expect(chipFor('status', null, 'select')).toBeNull()
    expect(chipFor('status', undefined, 'select')).toBeNull()
    expect(chipFor('bad name', 'x', 'text')).toBeNull()
    expect(chipFor('a;DROP', 'x', 'text')).toBeNull()
    expect(chipFor('1abc', 'x', 'text')).toBeNull()
  })
})

describe('bucket ranges', () => {
  it('turns each bucket into a half-open UTC interval', () => {
    expect(bucketRange('hour', '2026-11-18 09:00')).toEqual(['2026-11-18T09:00:00Z', '2026-11-18T10:00:00Z'])
    expect(bucketRange('day', '2026-11-30')).toEqual(['2026-11-30T00:00:00Z', '2026-12-01T00:00:00Z'])
    expect(bucketRange('week', '2026-11-16')).toEqual(['2026-11-16T00:00:00Z', '2026-11-23T00:00:00Z'])
    expect(bucketRange('month', '2026-12')).toEqual(['2026-12-01T00:00:00Z', '2027-01-01T00:00:00Z'])
    expect(bucketRange('year', '2026')).toEqual(['2026-01-01T00:00:00Z', '2027-01-01T00:00:00Z'])
  })
  it('refuses buckets it cannot parse instead of inventing an interval', () => {
    for (const [unit, bucket] of [['day', 'garbage'], ['day', '2026-13-45x'], ['month', '2026'], ['hour', '2026-11-18'], ['year', 'abcd'], ['minute', '2026-11-18'], ['day', '']] as const) {
      expect(bucketRange(unit, bucket), `${unit} ${bucket}`).toBeNull()
    }
  })
  it('bucketChip builds a two-sided range and validates the field', () => {
    const chip = bucketChip('placed', 'day', '2026-11-16')!
    expect(chip.expr).toBe('placed>="2026-11-16T00:00:00Z" && placed<"2026-11-17T00:00:00Z"')
    expect(chip.label).toBe('placed=2026-11-16')
    expect(chip.id).toBe('placed@day:2026-11-16')
    expect(bucketChip('bad name', 'day', '2026-11-16')).toBeNull()
    expect(bucketChip('placed', 'day', 'nope')).toBeNull()
  })
})

describe('binChip', () => {
  it('builds [start, start+step)', () => {
    expect(binChip('amount', 10, 10)).toEqual({ id: 'amount@bin:10', label: 'amount∈[10, 20)', expr: 'amount>=10 && amount<20' })
    expect(binChip('amount', '0', 0.5)?.expr).toBe('amount>=0 && amount<0.5')
    expect(binChip('amount', -10, 10)?.expr).toBe('amount>=-10 && amount<0')
  })
  it('rejects invalid input', () => {
    expect(binChip('amount', 'x', 10)).toBeNull()
    expect(binChip('amount', 1, 0)).toBeNull()
    expect(binChip('amount', 1, -1)).toBeNull()
    expect(binChip('bad name', 1, 10)).toBeNull()
  })
})

describe('pushHistory', () => {
  it('keeps the newest first, dedupes and caps at five', () => {
    let h: string[] = []
    for (const f of ['a', 'b', 'c', 'd', 'e', 'f']) h = pushHistory(h, f)
    expect(h).toEqual(['f', 'e', 'd', 'c', 'b'])
    expect(pushHistory(h, 'd')).toEqual(['d', 'f', 'e', 'c', 'b'])
  })
  it('ignores blank filters', () => {
    expect(pushHistory(['a'], '   ')).toEqual(['a'])
    expect(pushHistory(['a'], '')).toEqual(['a'])
  })
  it('trims before comparing', () => {
    expect(pushHistory(['a'], '  a  ')).toEqual(['a'])
  })
})
