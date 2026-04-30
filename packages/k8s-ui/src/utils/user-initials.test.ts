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
    // Bugbot regression: the v2 fallback returned `localPart.slice(0,2)`
    // which leaked the leading separator into the avatar circle as
    // ".U", "-A", "_O" etc.
    expect(computeUserInitials('.user')).toBe('US')
    expect(computeUserInitials('-admin')).toBe('AD')
    expect(computeUserInitials('_ops')).toBe('OP')
  })

  it('takes only the first letter of a single segment with trailing separator', () => {
    // Reviewer regression: the v2 docstring said "if local-part
    // contains separators, use the first letter of each segment",
    // but the code branched on segments.length >= 2 AFTER
    // filter(Boolean), so 'mary.' (one segment) used the
    // whole-localPart fallback and returned 'MA'. The contract is
    // that single-segment inputs should fall back to leading
    // letters of the SEGMENT, not the localPart.
    expect(computeUserInitials('mary.')).toBe('MA')
    expect(computeUserInitials('.mary')).toBe('MA')
  })

  it('returns empty for inputs with no letters', () => {
    // Docstring contract: "Returns '' when no letters survive".
    // v2 returned '..', '12', '_' for these inputs.
    expect(computeUserInitials('..')).toBe('')
    expect(computeUserInitials('123')).toBe('')
    expect(computeUserInitials('_')).toBe('')
    expect(computeUserInitials('---')).toBe('')
  })

  it('skips digits and punctuation interleaved with letters', () => {
    expect(computeUserInitials('m1k')).toBe('MK')
    expect(computeUserInitials('a$b$c')).toBe('AB')
  })
})
