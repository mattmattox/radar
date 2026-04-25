import type { ReactNode } from 'react';

// Shared status-pill + status-dot vocabulary. Before this, five pages
// each hand-rolled their own emerald/amber/rose/slate → `bg-*-50` ladder.
// CLAUDE.md's "Status-pill vocabulary" section codifies these 5 tones
// as cross-page meaning — this component enforces the invariant via
// types so future pages can't drift.
//
// Tones and their semantics (keep stable — pages across the app rely
// on the meanings, not just the colors):
//   ok       — live / active / healthy (emerald)
//   warn     — expiring soon / idle / degraded (amber)
//   danger   — revoked / disconnected / unhealthy (rose)
//   alert    — between warn and danger; high-urgency ops state (orange)
//   neutral  — informational scope counter (slate, always muted)
//   muted    — terminal state, no action needed (slate, even more muted)
//
// `alert` vs `danger`: Problems page uses "critical/high/medium". We
// map critical → danger, high → alert, medium → warn so the visual
// urgency progression reads right.

export type StatusTone = 'ok' | 'warn' | 'danger' | 'alert' | 'neutral' | 'muted';

const PILL_CLASS: Record<StatusTone, string> = {
  ok:
    'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-900',
  warn:
    'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-900',
  danger:
    'bg-rose-50 text-rose-700 ring-rose-200 dark:bg-rose-950/40 dark:text-rose-300 dark:ring-rose-900',
  alert:
    'bg-orange-50 text-orange-700 ring-orange-200 dark:bg-orange-950/40 dark:text-orange-300 dark:ring-orange-900',
  neutral:
    'bg-slate-100 text-slate-600 ring-slate-200 dark:bg-slate-800/60 dark:text-slate-300 dark:ring-slate-700',
  muted:
    'bg-slate-100 text-slate-500 ring-slate-200 dark:bg-slate-800/40 dark:text-slate-400 dark:ring-slate-700',
};

const DOT_CLASS: Record<StatusTone, string> = {
  ok: 'bg-emerald-500',
  warn: 'bg-amber-500',
  danger: 'bg-rose-500',
  alert: 'bg-orange-500',
  neutral: 'bg-slate-400',
  muted: 'bg-slate-400',
};

// mapHealthToTone: the handful of health vocabularies used by fleet
// endpoints (healthy|degraded|unhealthy|unknown; danger|warning; etc.)
// normalize to one tone vocabulary here so callers get a stable mapping.
export function mapHealthToTone(health: string): StatusTone {
  switch (health) {
    case 'healthy':
    case 'ok':
      return 'ok';
    case 'degraded':
    case 'warning':
    case 'warn':
      return 'warn';
    case 'unhealthy':
    case 'danger':
    case 'critical':
      return 'danger';
    case 'high':
      return 'alert';
    case 'medium':
      return 'warn';
    default:
      return 'neutral';
  }
}

export interface StatusPillProps {
  tone: StatusTone;
  children: ReactNode;
  /** Render a leading colored dot. Default true. */
  withDot?: boolean;
  /** Extra Tailwind classes (useful for size overrides). */
  className?: string;
  /** Optional title shown on hover. */
  title?: string;
}

export function StatusPill({ tone, children, withDot = true, className = '', title }: StatusPillProps) {
  return (
    <span
      title={title}
      className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${PILL_CLASS[tone]} ${className}`}
    >
      {withDot && <span className={`h-1.5 w-1.5 rounded-full ${DOT_CLASS[tone]}`} aria-hidden />}
      {children}
    </span>
  );
}

export interface StatusDotProps {
  tone: StatusTone;
  /** Size in Tailwind units. Default is 1.5 (h-1.5/w-1.5). */
  size?: 'xs' | 'sm' | 'md';
  className?: string;
}

const DOT_SIZE: Record<NonNullable<StatusDotProps['size']>, string> = {
  xs: 'h-1 w-1',
  sm: 'h-1.5 w-1.5',
  md: 'h-2 w-2',
};

export function StatusDot({ tone, size = 'sm', className = '' }: StatusDotProps) {
  return (
    <span
      className={`inline-block rounded-full ${DOT_SIZE[size]} ${DOT_CLASS[tone]} ${className}`}
      aria-hidden
    />
  );
}
