import { HEALTH_META } from '../../utils/applications'

/** Ready/desired progress bar shared by the Applications list and detail —
 *  colors come from HEALTH_META so the bar can't drift from the health system. */
export function ReadyBar({ ready, desired, width = 'w-12' }: { ready: number; desired: number; width?: string }) {
  if (desired <= 0) {
    return <span className="font-mono text-xs tabular-nums text-theme-text-tertiary">—</span>
  }
  const pct = Math.min(100, Math.round((ready / desired) * 100))
  const ok = ready >= desired
  const bar = ok ? HEALTH_META.healthy.bar : ready === 0 ? HEALTH_META.unhealthy.bar : HEALTH_META.degraded.bar
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`inline-block h-1.5 ${width} rounded-full bg-theme-hover`}>
        <span className={`block h-1.5 rounded-full ${bar}`} style={{ width: `${pct}%` }} />
      </span>
      <span className={`font-mono text-xs tabular-nums ${ok ? 'text-theme-text-secondary' : HEALTH_META.unhealthy.text}`}>{ready}/{desired || '—'}</span>
    </span>
  )
}
