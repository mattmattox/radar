import { describe, it, expect } from 'vitest'
import { computePodTransientCount, computeDeploymentsProgressing } from './count-reconcile'

// SKY-827 bugs 14 + 18: the Home dashboard's ring labels (Pods=235,
// Deployments=100) didn't agree with their breakdown (88 + 9 = 97
// ≠ 100; categorised pods < total pods). Backend reports the
// buckets independently, so the frontend has to derive the
// remainder for the rings to sum to the label and for the user not
// to perceive missing data.
//
// These two helpers are pure so the derivation rule is unit-tested
// in isolation. If they ever return negative we'd render a
// bogus segment, so the Math.max(0, ...) defensive clamp is also
// pinned here.

describe('computePodTransientCount', () => {
  it('returns the gap between cluster total and categorised buckets', () => {
    expect(computePodTransientCount({ total: 235, healthy: 200, warning: 10, error: 20 })).toBe(5)
  })

  it('returns 0 when categorised pods equal the total', () => {
    expect(computePodTransientCount({ total: 100, healthy: 80, warning: 5, error: 15 })).toBe(0)
  })

  it('clamps to 0 when categorised pods exceed the total (defensive against backend overcounting)', () => {
    expect(computePodTransientCount({ total: 50, healthy: 40, warning: 10, error: 5 })).toBe(0)
  })

  it('returns the total when no pods are categorised yet', () => {
    expect(computePodTransientCount({ total: 12, healthy: 0, warning: 0, error: 0 })).toBe(12)
  })

  it('handles a zero-pod cluster', () => {
    expect(computePodTransientCount({ total: 0, healthy: 0, warning: 0, error: 0 })).toBe(0)
  })
})

describe('computeDeploymentsProgressing', () => {
  it('returns the gap between total and (available + unavailable)', () => {
    // The exact bug from SKY-827 #18.
    expect(computeDeploymentsProgressing({ total: 100, available: 88, unavailable: 9 })).toBe(3)
  })

  it('returns 0 when all deployments are accounted for', () => {
    expect(computeDeploymentsProgressing({ total: 50, available: 45, unavailable: 5 })).toBe(0)
  })

  it('clamps to 0 when buckets overcount the total (defensive)', () => {
    expect(computeDeploymentsProgressing({ total: 10, available: 8, unavailable: 5 })).toBe(0)
  })

  it('returns the total when nothing is yet available or unavailable', () => {
    expect(computeDeploymentsProgressing({ total: 7, available: 0, unavailable: 0 })).toBe(7)
  })
})
