import { describe, expect, it } from 'vitest'
import { buildQuery, formatDateFa, formatRial, toFaDigits } from './utils'

describe('toFaDigits', () => {
  it('converts ASCII digits to Persian', () => {
    expect(toFaDigits('1234567890')).toBe('۱۲۳۴۵۶۷۸۹۰')
  })

  it('leaves non-digits untouched', () => {
    expect(toFaDigits('abc-۱۲')).toBe('abc-۱۲')
  })

  it('accepts numbers', () => {
    expect(toFaDigits(42)).toBe('۴۲')
  })
})

describe('formatRial', () => {
  it('converts rials to toman (÷10) with Persian digits', () => {
    expect(formatRial(120000)).toBe('۱۲,۰۰۰ تومان')
  })

  it('rounds fractional toman', () => {
    expect(formatRial(12345)).toBe('۱,۲۳۵ تومان')
  })

  it('formats zero', () => {
    expect(formatRial(0)).toBe('۰ تومان')
  })
})

describe('buildQuery', () => {
  it('skips undefined and empty values', () => {
    expect(buildQuery({ a: 1, b: undefined, c: '', d: 'x' })).toBe('?a=1&d=x')
  })

  it('returns empty string when nothing remains', () => {
    expect(buildQuery({ a: undefined, b: '' })).toBe('')
  })

  it('encodes values', () => {
    expect(buildQuery({ q: 'علی رضایی' })).toBe('?q=%D8%B9%D9%84%DB%8C+%D8%B1%D8%B6%D8%A7%DB%8C%DB%8C')
  })
})

describe('formatDateFa', () => {
  it('formats a valid ISO date with fa-IR locale', () => {
    const out = formatDateFa('2026-10-14T10:30:00Z')
    expect(out.length).toBeGreaterThan(4)
    expect(out).not.toBe('2026-10-14T10:30:00Z')
  })

  it('falls back to the raw input on invalid dates', () => {
    expect(formatDateFa('not-a-date')).toBe('not-a-date')
  })
})
