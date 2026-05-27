import { useMemo, useState, type ReactNode } from 'react'
import { ChevronRight, ExternalLink, Search, ShieldCheck, X } from 'lucide-react'
import { ClusterName, EmptyState, FilterPill } from '../ui'
import type { CheckMeta } from '../audit'
import { ActionItemDrawer } from './ActionItemDrawer'
import { CHECK_SEVERITIES, CHECK_SEVERITY_RANK, type CheckActionItem, type CheckSeverity, type EffectiveCheckFinding, type CheckResourceRef } from './types'
import {
  SEVERITY_BADGE_CLASS,
  SEVERITY_FILL_CLASS,
  SEVERITY_LABEL,
  SEVERITY_RAIL_CLASS,
  SEVERITY_TEXT_CLASS,
  categoryBadgeClass,
} from './severity'

const CATEGORIES: readonly string[] = ['Security', 'Reliability', 'Efficiency']

export interface ActionItemsViewProps {
  /** Action items, typically flattened across the fleet by the host. */
  items: CheckActionItem[]
  /** Check metadata (id → meta) for the drawer's title/description/remediation. */
  checks: Record<string, CheckMeta>
  /** True when at least one source returned audit data — distinguishes "clean"
   *  from "nothing audited / everything errored". */
  anyData: boolean
  /** Resolve a deep-link href for a resource (host-specific routing). Omit to
   *  render non-link text. */
  resourceHref?: (ref: CheckResourceRef) => string
  /** Display label for an item's source cluster. Omit (or return falsy) to hide
   *  the cluster line — e.g. single-cluster OSS. */
  clusterLabel?: (item: CheckActionItem) => string | undefined
  /** Empty-state CTA shown when there's no data (host-specific: connect a
   *  cluster vs run an audit). */
  emptyAction?: ReactNode
}

export function ActionItemsView({ items, checks, anyData, resourceHref, clusterLabel, emptyAction }: ActionItemsViewProps) {
  const [severityFilter, setSeverityFilter] = useState<Set<CheckSeverity>>(new Set())
  const [categoryFilter, setCategoryFilter] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [detailId, setDetailId] = useState<string | null>(null)

  const { totals, totalFindings, clusterCount } = useMemo(() => {
    const totals: Record<CheckSeverity, number> = { critical: 0, high: 0, medium: 0, low: 0 }
    const clusters = new Set<string>()
    let totalFindings = 0
    for (const it of items) {
      totals[it.effectiveSeverity] += 1
      totalFindings += it.affectedFindings
      clusters.add(it.subject.cluster_id)
    }
    return { totals, totalFindings, clusterCount: clusters.size }
  }, [items])

  const searchLower = search.toLowerCase()
  const filtered = useMemo(() => {
    const out = items.filter((it) => {
      if (severityFilter.size > 0 && !severityFilter.has(it.effectiveSeverity)) return false
      if (categoryFilter.size > 0 && !categoryFilter.has(it.category)) return false
      if (searchLower) {
        const hay = `${it.title} ${it.checkID} ${it.message} ${it.subject.namespace} ${it.subject.name}`.toLowerCase()
        if (!hay.includes(searchLower)) return false
      }
      return true
    })
    // Worst-first across the whole queue (severity, then blast radius, then title).
    return out.sort((a, b) => {
      const r = CHECK_SEVERITY_RANK[b.effectiveSeverity] - CHECK_SEVERITY_RANK[a.effectiveSeverity]
      if (r !== 0) return r
      if (b.affectedResources !== a.affectedResources) return b.affectedResources - a.affectedResources
      return a.title.localeCompare(b.title)
    })
  }, [items, severityFilter, categoryFilter, searchLower])

  const toggle = <T,>(setter: React.Dispatch<React.SetStateAction<Set<T>>>, v: T) =>
    setter((prev) => {
      const next = new Set(prev)
      if (next.has(v)) next.delete(v)
      else next.add(v)
      return next
    })

  const hasFilters = severityFilter.size > 0 || categoryFilter.size > 0 || search !== ''
  const clearAll = () => {
    setSeverityFilter(new Set())
    setCategoryFilter(new Set())
    setSearch('')
  }

  const detail = detailId ? filtered.find((it) => it.id === detailId) ?? items.find((it) => it.id === detailId) ?? null : null

  return (
    <div className="flex flex-col gap-4">
      {/* Triage header: distribution bar + filter chips + search. */}
      <div className="flex flex-col gap-3.5 rounded-2xl border border-theme-border bg-theme-surface p-4 shadow-theme-sm">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-semibold tabular-nums text-theme-text-primary">{items.length}</span>
            <span className="text-sm text-theme-text-secondary">
              {items.length === 1 ? 'action item' : 'action items'}
              {totalFindings > items.length && <span className="text-theme-text-tertiary"> · {totalFindings} findings</span>}
              {clusterCount > 1 && <span className="text-theme-text-tertiary"> · {clusterCount} clusters</span>}
            </span>
          </div>
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-theme-text-tertiary" />
            <input
              type="text"
              placeholder="Search action items…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-64 rounded-lg border border-theme-border-light bg-theme-base py-1.5 pl-9 pr-8 text-sm text-theme-text-primary placeholder-theme-text-disabled focus:outline-none focus:ring-2 focus:ring-[var(--color-radar-accent)]"
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch('')}
                aria-label="Clear search"
                className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-0.5 text-theme-text-tertiary hover:text-theme-text-primary"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </div>

        <SeverityBar totals={totals} />

        <div className="flex flex-wrap items-center gap-1.5">
          {CHECK_SEVERITIES.map((s) => (
            <SeverityChip key={s} severity={s} count={totals[s]} active={severityFilter.has(s)} onClick={() => toggle(setSeverityFilter, s)} />
          ))}
          <span className="mx-1.5 h-5 w-px bg-theme-border" />
          {CATEGORIES.map((c) => (
            <FilterPill key={c} label={c} active={categoryFilter.has(c)} onClick={() => toggle(setCategoryFilter, c)} />
          ))}
        </div>
      </div>

      {filtered.length === 0 ? (
        hasFilters ? (
          <EmptyState
            tone="filtered"
            variant="card"
            headline="No action items match the current filters"
            body="Clear a filter to see more of the queue."
            action={
              <button
                type="button"
                onClick={clearAll}
                className="badge badge-sm border border-theme-border bg-theme-elevated text-theme-text-primary transition-colors hover:bg-theme-hover"
              >
                Clear all filters
              </button>
            }
          />
        ) : anyData ? (
          <EmptyState
            tone="healthy"
            variant="card"
            icon={ShieldCheck}
            headline="Nothing to remediate"
            body="Every audited resource passed its checks."
          />
        ) : (
          <EmptyState headline="No check data yet" body="Run an audit to populate the remediation queue." action={emptyAction} />
        )
      ) : (
        <ol className="flex flex-col gap-1.5">
          {filtered.map((item) => (
            <ActionItemRow
              key={item.id}
              item={item}
              clusterLabel={clusterLabel}
              expanded={expanded.has(item.id)}
              onToggle={() => toggle(setExpanded, item.id)}
              onOpen={() => setDetailId(item.id)}
              resourceHref={resourceHref}
            />
          ))}
        </ol>
      )}

      {detail && (
        <ActionItemDrawer item={detail} meta={checks[detail.checkID]} resourceHref={resourceHref} clusterLabel={clusterLabel} onClose={() => setDetailId(null)} />
      )}
    </div>
  )
}

function SeverityBar({ totals }: { totals: Record<CheckSeverity, number> }) {
  const sum = CHECK_SEVERITIES.reduce((n, s) => n + totals[s], 0)
  return (
    <div className="flex h-1.5 overflow-hidden rounded-full bg-theme-elevated" role="img" aria-label="Severity distribution">
      {sum === 0
        ? null
        : CHECK_SEVERITIES.map((s) =>
            totals[s] > 0 ? (
              <div key={s} className={`${SEVERITY_FILL_CLASS[s]} transition-[width] duration-500 ease-out`} style={{ width: `${(totals[s] / sum) * 100}%` }} />
            ) : null,
          )}
    </div>
  )
}

function SeverityChip({ severity, count, active, onClick }: { severity: CheckSeverity; count: number; active: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={[
        'group inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors',
        active ? 'border-theme-border bg-theme-elevated text-theme-text-primary' : 'border-transparent text-theme-text-secondary hover:bg-theme-hover/60',
      ].join(' ')}
    >
      <span className={`h-2 w-2 rounded-full ${SEVERITY_FILL_CLASS[severity]} ${count === 0 ? 'opacity-30' : ''}`} />
      <span className={`font-semibold tabular-nums ${count > 0 ? SEVERITY_TEXT_CLASS[severity] : 'text-theme-text-tertiary'}`}>{count}</span>
      <span>{SEVERITY_LABEL[severity]}</span>
    </button>
  )
}

function ActionItemRow({
  item,
  clusterLabel,
  expanded,
  onToggle,
  onOpen,
  resourceHref,
}: {
  item: CheckActionItem
  clusterLabel?: (item: CheckActionItem) => string | undefined
  expanded: boolean
  onToggle: () => void
  onOpen: () => void
  resourceHref?: (ref: CheckResourceRef) => string
}) {
  // Only the environment factor is genuinely additive on the row — severity is
  // the badge, category is the tag, blast_radius is the resource count.
  const extraFactors = item.priorityFactors.filter(
    (f) => f.weight > 0 && f.key !== 'severity' && f.key !== 'category' && f.key !== 'blast_radius',
  )
  const cluster = clusterLabel?.(item)

  return (
    <li className="overflow-hidden rounded-xl border border-theme-border bg-theme-surface shadow-theme-sm">
      <div
        role="button"
        tabIndex={0}
        onClick={onOpen}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onOpen()
          }
        }}
        className={`group flex cursor-pointer items-center gap-3 border-l-2 py-3 pl-3 pr-4 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-radar-accent)]/40 ${SEVERITY_RAIL_CLASS[item.effectiveSeverity]}`}
      >
        <button
          type="button"
          aria-label={expanded ? 'Collapse affected resources' : 'Show affected resources'}
          aria-expanded={expanded}
          onClick={(e) => {
            e.stopPropagation()
            onToggle()
          }}
          className="-my-1 shrink-0 rounded-md p-1 text-theme-text-tertiary transition-colors hover:bg-theme-hover hover:text-theme-text-secondary"
        >
          <ChevronRight className={`h-4 w-4 transition-transform duration-200 ${expanded ? 'rotate-90' : ''}`} />
        </button>

        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium text-theme-text-primary">{item.title}</span>
            <span className={`badge-sm shrink-0 text-[10px] ${categoryBadgeClass(item.category)}`}>{item.category}</span>
          </div>
          <div className="flex min-w-0 items-center gap-1.5 text-xs text-theme-text-tertiary">
            {cluster ? (
              <>
                <span className="max-w-[180px] truncate">
                  <ClusterName name={cluster} />
                </span>
                <span aria-hidden>·</span>
              </>
            ) : null}
            <span className="shrink-0 font-medium text-theme-text-secondary tabular-nums">
              {item.affectedResources} {item.affectedResources === 1 ? 'resource' : 'resources'}
            </span>
            {extraFactors.map((f) => (
              <span key={f.key} className="hidden shrink-0 items-center gap-1 capitalize sm:inline-flex">
                <span aria-hidden>·</span>
                {f.detail || f.label}
              </span>
            ))}
          </div>
        </div>

        <span className={`badge-sm shrink-0 text-[10px] font-semibold ${SEVERITY_BADGE_CLASS[item.effectiveSeverity]}`}>{SEVERITY_LABEL[item.effectiveSeverity]}</span>
        <ExternalLink className="h-3.5 w-3.5 shrink-0 text-theme-text-tertiary opacity-0 transition-opacity group-hover:opacity-100" />
      </div>

      <div className="grid transition-[grid-template-rows] duration-200 ease-out" style={{ gridTemplateRows: expanded ? '1fr' : '0fr' }}>
        <div className="overflow-hidden">
          <ul className="flex flex-col gap-px border-t border-theme-border bg-theme-base/40 px-3 py-2 pl-12">
            {item.findings.map((f) => (
              <FindingLine key={`${f.resource.group}/${f.resource.kind}/${f.resource.namespace}/${f.resource.name}`} finding={f} resourceHref={resourceHref} />
            ))}
          </ul>
        </div>
      </div>
    </li>
  )
}

function FindingLine({ finding, resourceHref }: { finding: EffectiveCheckFinding; resourceHref?: (ref: CheckResourceRef) => string }) {
  const r = finding.resource
  const body = (
    <>
      <span className="shrink-0 font-mono text-[11px] uppercase tracking-wide text-theme-text-tertiary">{r.kind}</span>
      <span className={`truncate font-medium ${resourceHref ? 'text-[var(--color-radar-accent)]' : 'text-theme-text-primary'}`}>
        {r.namespace ? `${r.namespace} / ` : ''}
        {r.name}
      </span>
      {resourceHref && <ExternalLink className="h-3 w-3 shrink-0 text-theme-text-tertiary opacity-0 transition-opacity group-hover/f:opacity-100" />}
      <span className="ml-1 hidden truncate text-xs text-theme-text-tertiary md:inline">{finding.message}</span>
    </>
  )
  return (
    <li>
      {resourceHref ? (
        <a href={resourceHref(r)} className="group/f flex items-center gap-2 rounded-md px-2 py-1 text-sm transition-colors hover:bg-theme-hover/60">
          {body}
        </a>
      ) : (
        <span className="flex items-center gap-2 rounded-md px-2 py-1 text-sm">{body}</span>
      )}
    </li>
  )
}
