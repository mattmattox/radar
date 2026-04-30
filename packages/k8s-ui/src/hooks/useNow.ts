import { useEffect, useState } from 'react'

/**
 * Pure predicate for whether `useNow` should schedule a tick for the
 * given interval. Extracted so the scheduling rule (null and
 * non-positive intervals opt out; anything > 0 ticks) can be
 * unit-tested without a React renderer.
 */
export function shouldScheduleNow(intervalMs: number | null): boolean {
  if (intervalMs === null) return false
  if (intervalMs <= 0) return false
  return true
}

/**
 * Pure scheduler used by `useNow`. Given the timer primitives, the
 * interval, and a setter, it installs an interval that calls
 * `setNow(nowFn())` every `intervalMs` and returns a cleanup
 * function — or installs nothing and returns a no-op for opt-out
 * intervals.
 *
 * Hoisted so the hook's full effect body (the part that's actually
 * worth testing — opt-out vs schedule, the cleanup contract, the
 * value passed to setNow) is unit-testable end-to-end without
 * needing a React renderer (this package's vitest config has
 * neither @testing-library/react nor jsdom).
 */
export function scheduleNowTicks(
  intervalMs: number | null,
  setNow: (n: number) => void,
  timers: {
    setInterval: (cb: () => void, ms: number) => unknown
    clearInterval: (id: unknown) => void
    now: () => number
  },
): () => void {
  if (!shouldScheduleNow(intervalMs)) {
    return () => {}
  }
  const id = timers.setInterval(() => setNow(timers.now()), intervalMs as number)
  return () => timers.clearInterval(id)
}

/**
 * Returns the current wall-clock time, refreshed every `intervalMs`.
 *
 * Use this when you have UI that derives a relative time string
 * (e.g. "Updated 8s", "24d") from a fixed timestamp. Without it,
 * the relative label only changes when the parent re-renders for
 * an unrelated reason — which makes the label feel frozen, and
 * worse: the next unrelated re-render makes it jump forward by
 * however many seconds passed in silence. Users perceived that as
 * a "data re-fetch was triggered" because the displayed age
 * suddenly updated.
 *
 * The interval is opt-in per call site so cells in tight tables
 * don't pay the cost of a 1Hz tick when they only need a 60Hz
 * update for "minutes" granularity.
 *
 * @param intervalMs how often to advance the clock. Defaults to
 *                   1000ms. Pass `null` to disable ticking
 *                   (returns the time at mount and never updates).
 * @returns the current `Date.now()` value.
 */
export function useNow(intervalMs: number | null = 1000): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    // Use globalThis for the timer primitives so the hook works
    // under SSR / Node-based tests too. In browsers these are the
    // same as window.setInterval; in Node, `window` is undefined.
    return scheduleNowTicks(intervalMs, setNow, {
      setInterval: globalThis.setInterval as (cb: () => void, ms: number) => unknown,
      clearInterval: globalThis.clearInterval as (id: unknown) => void,
      now: Date.now,
    })
  }, [intervalMs])

  return now
}
