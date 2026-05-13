import { describe, it, expect } from 'vitest'
import { computeUserInitials } from './user-initials'

// Pin the contract that the avatar circle never tries to render a
// non-letter glyph: previous implementations either produced
// silhouettes for separator-free usernames OR leaked separator
// characters into the result (e.g. ".U" for ".user").

describe('computeUserInitials', () => {
  it('uses segment initials when separators are present', () => {
    expect(computeUserInitials('mary.kohli')).toBe('MK')
    expect(computeUserInitials('mary_kohli')).toBe('MK')
    expect(computeUserInitials('mary-kohli')).toBe('MK')
  })

  it('caps segment initials at 2 even with many separators', () => {
    expect(computeUserInitials('a.b.c.d')).toBe('AB')
  })

  it('falls back to leading letters when no separators', () => {
    expect(computeUserInitials('mkohli')).toBe('MK')
    expect(computeUserInitials('alice')).toBe('AL')
  })

  it('returns a single letter for single-character usernames', () => {
    expect(computeUserInitials('a')).toBe('A')
  })

  it('strips the @-domain before computing', () => {
    expect(computeUserInitials('mary.kohli@example.com')).toBe('MK')
    expect(computeUserInitials('mkohli@example.com')).toBe('MK')
  })

  it('uppercases the result', () => {
    expect(computeUserInitials('alice')).toBe('AL')
    expect(computeUserInitials('ALICE')).toBe('AL')
    expect(computeUserInitials('aLiCe')).toBe('AL')
  })

  it('returns empty string for null/undefined/empty inputs', () => {
    expect(computeUserInitials(null)).toBe('')
    expect(computeUserInitials(undefined)).toBe('')
    expect(computeUserInitials('')).toBe('')
  })

  it('handles consecutive separators without producing empty segments', () => {
    expect(computeUserInitials('mary..kohli')).toBe('MK')
    expect(computeUserInitials('mary__kohli')).toBe('MK')
  })

  it('handles email-only usernames with @ as the first character', () => {
    expect(computeUserInitials('@example.com')).toBe('')
  })

  it('does not include separator characters in the fallback', () => {
    // Leading/trailing separators must not leak into the avatar
    // circle as ".U", "-A", "_O" — non-letters get filtered before
    // the slice, not after.
    expect(computeUserInitials('.user')).toBe('US')
    expect(computeUserInitials('-admin')).toBe('AD')
    expect(computeUserInitials('_ops')).toBe('OP')
  })

  it('takes leading letters of the segment, not the whole localPart, for trailing/leading separator inputs', () => {
    // Single-segment inputs must fall back to leading letters of
    // the SEGMENT, not the raw localPart — otherwise `'mary.'`
    // returns `'MA'` from the wrong slice and inconsistency with
    // `'mary.kohli'` shows up only on partial inputs.
    expect(computeUserInitials('mary.')).toBe('MA')
    expect(computeUserInitials('.mary')).toBe('MA')
  })

  it('returns empty for inputs with no letters', () => {
    // The avatar circle has no glyph to render for separator-only,
    // digit-only, or punctuation-only inputs — return `''` so the
    // caller can fall back to a silhouette.
    expect(computeUserInitials('..')).toBe('')
    expect(computeUserInitials('123')).toBe('')
    expect(computeUserInitials('_')).toBe('')
    expect(computeUserInitials('---')).toBe('')
  })

  it('drops leading whitespace before computing initials', () => {
    // Leading whitespace must not leak into the avatar circle as
    // a pair of blank glyphs — the truthy `'  '` would defeat the
    // caller's `{initials || <silhouette>}` fallback.
    expect(computeUserInitials('  alice')).toBe('AL')
    expect(computeUserInitials('\talice')).toBe('AL')
  })

  it('skips digits and punctuation interleaved with letters', () => {
    expect(computeUserInitials('m1k')).toBe('MK')
    expect(computeUserInitials('a$b$c')).toBe('AB')
  })
})
