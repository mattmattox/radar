import { useEffect } from 'react'
import { createPortal } from 'react-dom'
import { X, ExternalLink, Wrench, ArrowRight, Layers } from 'lucide-react'
import { ClusterName } from '../ui'
import type { CheckMeta } from '../audit'
import type { CheckActionItem, EffectiveCheckFinding, CheckResourceRef } from './types'
import { SEVERITY_BADGE_CLASS, SEVERITY_FILL_CLASS, SEVERITY_GLOW_CLASS, SEVERITY_LABEL } from './severity'

// The remediation cockpit for one action item. A right-side slide-over portaled
// to document.body. Host-agnostic: deep-links come from `resourceHref` and the
// cluster label from `clusterLabel`, so Hub (cross-SPA links, fleet cluster
// names) and OSS (in-app links, single cluster) both drive it.

export interface ActionItemDrawerProps {
  item: CheckActionItem
  meta?: CheckMeta
  /** Resolve a deep-link href for a resource. Omit to render non-link text. */
  resourceHref?: (ref: CheckResourceRef) => string
  /** Display label for the item's source cluster. Omit (or return falsy) to
   *  hide the cluster line — e.g. single-cluster OSS. */
  clusterLabel?: (item: CheckActionItem) => string | undefined
  onClose: () => void
}

export function ActionItemDrawer({ item, meta, resourceHref, clusterLabel, onClose }: ActionItemDrawerProps) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  const rep = item.representativeFinding
  const sev = item.effectiveSeverity
  const fromOrgConfig = rep.state.source === 'org_config'
  const cluster = clusterLabel?.(item)

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex justify-end bg-theme-base/50 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="action-item-drawer-title"
    >
      <div className="flex h-full w-full max-w-xl flex-col border-l border-theme-border/80 bg-theme-surface shadow-2xl">
        {/* Severity-themed header band */}
        <div className={`relative shrink-0 px-5 py-4 ring-1 ring-inset ${SEVERITY_GLOW_CLASS[sev]}`}>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="absolute right-3 top-3 rounded-md p-1 text-theme-text-tertiary transition-colors hover:bg-theme-hover hover:text-theme-text-secondary"
          >
            <X className="h-5 w-5" />
          </button>
          <div className="flex flex-col gap-2 pr-8">
            <div className="flex items-center gap-2">
              <span className={`badge-sm text-[11px] font-semibold ${SEVERITY_BADGE_CLASS[sev]}`}>{SEVERITY_LABEL[sev]}</span>
              <span className="text-[11px] font-medium uppercase tracking-wide text-theme-text-tertiary">{item.category}</span>
            </div>
            <h2 id="action-item-drawer-title" className="text-lg font-semibold leading-snug text-theme-text-primary">
              {item.title}
            </h2>
            <div className="flex items-center gap-1.5 text-xs text-theme-text-secondary">
              <Layers className="h-3.5 w-3.5 text-theme-text-tertiary" />
              {cluster ? (
                <>
                  <ClusterName name={cluster} />
                  <span aria-hidden>·</span>
                </>
              ) : null}
              <span className="tabular-nums">
                {item.affectedResources} {item.affectedResources === 1 ? 'resource' : 'resources'}
                {item.affectedFindings !== item.affectedResources && ` · ${item.affectedFindings} findings`}
              </span>
            </div>
          </div>
        </div>

        <div className="flex flex-1 flex-col gap-5 overflow-y-auto px-5 py-5">
          {/* How to fix — the primary, actionable card. */}
          {meta?.remediation && (
            <section className="rounded-xl border border-[var(--color-radar-accent)]/25 bg-[var(--color-radar-accent)]/[0.06] p-3.5">
              <h3 className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-radar-accent)]">
                <Wrench className="h-3.5 w-3.5" /> How to fix
              </h3>
              <p className="text-sm leading-relaxed text-theme-text-primary">{meta.remediation}</p>
            </section>
          )}

          {meta?.description && (
            <section className="flex flex-col gap-1">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-theme-text-tertiary">What this checks</h3>
              <p className="text-sm leading-relaxed text-theme-text-secondary">{meta.description}</p>
            </section>
          )}

          {/* Severity story: detector reading → effective ladder value. */}
          <section className="flex flex-col gap-2">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-theme-text-tertiary">Severity</h3>
            <div className="flex items-center gap-3 rounded-xl border border-theme-border bg-theme-base px-3.5 py-3">
              <div className="flex flex-col">
                <span className="text-[10px] uppercase tracking-wide text-theme-text-tertiary">Detector</span>
                <span className="text-sm font-medium capitalize text-theme-text-secondary">{rep.originalSeverity}</span>
              </div>
              <ArrowRight className="h-4 w-4 text-theme-text-tertiary" />
              <div className="flex flex-col">
                <span className="text-[10px] uppercase tracking-wide text-theme-text-tertiary">Effective</span>
                <span className="text-sm font-semibold text-theme-text-primary">{SEVERITY_LABEL[sev]}</span>
              </div>
              <span className="flex-1" />
              <span
                className={[
                  'rounded-full px-2 py-0.5 text-[11px] font-medium',
                  fromOrgConfig
                    ? 'bg-[var(--color-radar-accent)]/10 text-[var(--color-radar-accent)]'
                    : 'bg-theme-elevated text-theme-text-tertiary',
                ].join(' ')}
                title={rep.state.reason || undefined}
              >
                {fromOrgConfig ? 'Org policy' : 'Detector default'}
              </span>
            </div>
          </section>

          <PriorityBreakdown item={item} />

          {/* Affected resources — exact identity + deep link per resource. */}
          <section className="flex flex-col gap-1.5">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-theme-text-tertiary">
              Affected resources <span className="tabular-nums">({item.affectedResources})</span>
            </h3>
            <ul className="flex flex-col gap-px">
              {item.findings.map((f) => (
                <ResourceRow
                  key={`${f.resource.group}/${f.resource.kind}/${f.resource.namespace}/${f.resource.name}`}
                  finding={f}
                  resourceHref={resourceHref}
                />
              ))}
            </ul>
            {resourceHref && (
              <a
                href={resourceHref(rep.resource)}
                className="mt-1 inline-flex w-fit items-center gap-1 text-xs font-medium text-[var(--color-radar-accent)] hover:underline"
              >
                Open representative resource <ArrowRight className="h-3 w-3" />
              </a>
            )}
          </section>
        </div>
      </div>
    </div>,
    document.body,
  )
}

// Visualized explainable priority: each weighted factor as a proportional bar,
// summing to the score. Zero-weight factors render as quiet context chips.
function PriorityBreakdown({ item }: { item: CheckActionItem }) {
  const weighted = item.priorityFactors.filter((f) => f.weight > 0)
  const context = item.priorityFactors.filter((f) => f.weight === 0)
  if (weighted.length === 0 && context.length === 0) return null
  const max = Math.max(...weighted.map((f) => f.weight), 1)
  const total = weighted.reduce((n, f) => n + f.weight, 0)

  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-theme-text-tertiary">Why this priority</h3>
        <span className="text-xs text-theme-text-tertiary">
          score <span className="font-semibold tabular-nums text-theme-text-secondary">{total}</span>
        </span>
      </div>
      <ul className="flex flex-col gap-2">
        {weighted.map((f) => (
          <li key={f.key} className="flex items-center gap-3 text-xs">
            <span className="w-32 shrink-0 truncate text-theme-text-secondary">
              {f.label}
              {f.detail ? <span className="text-theme-text-tertiary"> · {f.detail}</span> : null}
            </span>
            <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-theme-elevated">
              <span
                className="block h-full rounded-full bg-[var(--color-radar-accent)] transition-[width] duration-500 ease-out"
                style={{ width: `${(f.weight / max) * 100}%` }}
              />
            </span>
            <span className="w-8 shrink-0 text-right font-mono text-theme-text-tertiary tabular-nums">+{f.weight}</span>
          </li>
        ))}
      </ul>
      {context.length > 0 && (
        <div className="flex flex-wrap gap-1.5 pt-0.5">
          {context.map((f) => (
            <span key={f.key} className="rounded-md bg-theme-elevated px-2 py-0.5 text-[11px] text-theme-text-tertiary ring-1 ring-theme-border">
              {f.label}
              {f.detail ? `: ${f.detail}` : ''}
            </span>
          ))}
        </div>
      )}
    </section>
  )
}

function ResourceRow({
  finding,
  resourceHref,
}: {
  finding: EffectiveCheckFinding
  resourceHref?: (ref: CheckResourceRef) => string
}) {
  const r = finding.resource
  const body = (
    <>
      <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${SEVERITY_FILL_CLASS[finding.effectiveSeverity]}`} />
      <span className="shrink-0 font-mono text-[11px] uppercase tracking-wide text-theme-text-tertiary">{r.kind}</span>
      <span className={`truncate font-medium ${resourceHref ? 'text-[var(--color-radar-accent)]' : 'text-theme-text-primary'}`}>
        {r.namespace ? `${r.namespace} / ` : ''}
        {r.name}
      </span>
      {resourceHref && <ExternalLink className="h-3 w-3 shrink-0 text-theme-text-tertiary opacity-0 transition-opacity group-hover:opacity-100" />}
    </>
  )
  return (
    <li>
      {resourceHref ? (
        <a href={resourceHref(r)} className="group flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-theme-hover/60">
          {body}
        </a>
      ) : (
        <span className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm">{body}</span>
      )}
    </li>
  )
}
