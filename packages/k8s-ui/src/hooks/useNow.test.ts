import { describe, it, expect } from 'vitest'
import { useNow, shouldScheduleNow } from './useNow'

// We don't have @testing-library/react / jsdom in this package's
// vitest setup, so we can't invoke the hook through a renderer here.
// Instead we:
//   1. Import `useNow` so the test fails if the module fails to load
//      or the export shape changes (catches accidental signature
//      breaks in CI).
//   2. Pin the scheduling predicate `shouldScheduleNow` — extracted
//      from the hook precisely so the branch logic that decides
//      "tick or don't tick" is unit-testable without a renderer.
//
// (Cursor Bugbot pointed out that a previous iteration of this file
// inlined the branch logic without ever importing `useNow`, giving
// false coverage. This rewrite makes the test reflect what's actually
// shipped.)

describe('useNow', () => {
  it('exports a callable hook with the documented signature', () => {
    expect(typeof useNow).toBe('function')
    // useNow(intervalMs?) — single optional parameter.
    expect(useNow.length).toBeLessThanOrEqual(1)
  })
})

describe('shouldScheduleNow', () => {
  it('schedules when intervalMs is a positive number', () => {
    expect(shouldScheduleNow(1)).toBe(true)
    expect(shouldScheduleNow(1000)).toBe(true)
    expect(shouldScheduleNow(60_000)).toBe(true)
  })

  it('opts out when intervalMs is null', () => {
    expect(shouldScheduleNow(null)).toBe(false)
  })

  it('opts out when intervalMs is zero', () => {
    expect(shouldScheduleNow(0)).toBe(false)
  })

  it('opts out when intervalMs is negative', () => {
    expect(shouldScheduleNow(-1)).toBe(false)
    expect(shouldScheduleNow(-1000)).toBe(false)
  })
})
