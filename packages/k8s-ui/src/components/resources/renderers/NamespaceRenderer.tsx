import { Shield, Box, Users } from 'lucide-react'
import { clsx } from 'clsx'
import { Section, PropertyList, Property, ResourceLink } from '../../ui/drawer-components'
import type { RBACNamespaceResponse, RBACBindingWithSubjects, RBACSubject, ResourceRef } from '../../../types'
import { rbacKindBadgeClass } from '../../../utils/rbac-badges'

interface NamespaceRendererProps {
  data: any
  /**
   * RBAC summary for this namespace fetched from /api/rbac/namespace/{ns}.
   * Undefined when the host hasn't wired the fetch (RBAC section omitted).
   * Null when the fetch failed; section shows a tactful note.
   */
  rbacData?: RBACNamespaceResponse | null
  rbacLoading?: boolean
  rbacError?: Error | null
  onNavigate?: (ref: ResourceRef) => void
}

export function NamespaceRenderer({ data, rbacData, rbacLoading, rbacError, onNavigate }: NamespaceRendererProps) {
  const metadata = data.metadata || {}
  const status = data.status || {}
  const phase = status.phase
  const labels = metadata.labels || {}

  // Common signal labels: control-plane / cluster-wide / managed-by-* markers.
  // We surface them as a quick read since the generic labels section is
  // collapsed in the sidebar.
  const istioInjection = labels['istio-injection']
  const linkerdInjection = labels['linkerd.io/inject']
  const managedBy = labels['app.kubernetes.io/managed-by']

  return (
    <>
      <Section title="Status" icon={Box}>
        <PropertyList>
          <Property label="Phase" value={
            phase ? (
              <span className={clsx(
                phase === 'Active' && 'text-emerald-700 dark:text-emerald-400',
                phase === 'Terminating' && 'text-orange-700 dark:text-orange-400',
              )}>{phase}</span>
            ) : undefined
          } />
          {istioInjection && <Property label="Istio injection" value={istioInjection} />}
          {linkerdInjection && <Property label="Linkerd injection" value={linkerdInjection} />}
          {managedBy && <Property label="Managed by" value={managedBy} />}
        </PropertyList>
      </Section>

      {/* RBAC summary — only when host wired the fetch. */}
      {rbacData !== undefined && (
        <NamespaceRBACSection
          rbacData={rbacData}
          loading={!!rbacLoading}
          error={rbacError ?? null}
          onNavigate={onNavigate}
        />
      )}
    </>
  )
}

// ============================================================================
// NAMESPACE RBAC SECTION
// ============================================================================
// Answers "what RBAC is configured here" without forcing operators to pivot
// through individual SAs. We show two slices:
//   1. RoleBindings whose own namespace is this namespace (the most direct
//      "configured here" answer).
//   2. ClusterRoleBindings with at least one ServiceAccount subject in this
//      namespace (cluster-wide grants whose blast radius touches the
//      namespace — what an attacker compromising a workload here would
//      inherit beyond the local bindings).
// Cluster-wide bindings to wide groups (system:authenticated etc.) are
// excluded from #2 because they touch *every* namespace and would be noise.

function NamespaceRBACSection({
  rbacData,
  loading,
  error,
  onNavigate,
}: {
  rbacData: RBACNamespaceResponse | null
  loading: boolean
  error: Error | null
  onNavigate?: (ref: ResourceRef) => void
}) {
  if (loading) {
    return (
      <Section title="RBAC" icon={Shield}>
        <div className="text-sm text-theme-text-secondary">Loading RBAC summary…</div>
      </Section>
    )
  }
  if (error) {
    return (
      <Section title="RBAC" icon={Shield}>
        <div className="text-sm text-red-400">Could not load RBAC summary: {error.message}</div>
      </Section>
    )
  }
  if (!rbacData) return null

  const localBindings = rbacData.roleBindings ?? []
  const clusterBindings = rbacData.clusterRoleBindingsWithLocalSubject ?? []
  const totalBindings = localBindings.length + clusterBindings.length

  return (
    <>
      <Section title="RBAC" icon={Shield} defaultExpanded>
        <PropertyList>
          <Property label="ServiceAccounts" value={rbacData.serviceAccountCount} />
          <Property label="Bindings touching this namespace" value={totalBindings} />
        </PropertyList>
      </Section>

      <Section title={`RoleBindings (${localBindings.length})`} icon={Users} defaultExpanded={localBindings.length > 0}>
        {localBindings.length === 0 ? (
          <div className="text-sm text-theme-text-secondary">
            No RoleBindings are defined in this namespace.
          </div>
        ) : (
          <div className="space-y-2">
            {localBindings.map((b) => (
              <BindingSummaryRow key={b.binding.kind + '/' + b.binding.namespace + '/' + b.binding.name} entry={b} onNavigate={onNavigate} />
            ))}
          </div>
        )}
      </Section>

      {clusterBindings.length > 0 && (
        <Section
          title={`ClusterRoleBindings touching this namespace (${clusterBindings.length})`}
          icon={Users}
        >
          <div className="space-y-2">
            {clusterBindings.map((b) => (
              <BindingSummaryRow key={b.binding.kind + '/' + b.binding.namespace + '/' + b.binding.name} entry={b} onNavigate={onNavigate} />
            ))}
          </div>
        </Section>
      )}
    </>
  )
}

function BindingSummaryRow({
  entry,
  onNavigate,
}: {
  entry: RBACBindingWithSubjects
  onNavigate?: (ref: ResourceRef) => void
}) {
  const bindingKindPlural =
    entry.binding.kind === 'RoleBinding' ? 'rolebindings' : 'clusterrolebindings'
  const roleKindPlural =
    entry.binding.roleRef.kind === 'Role' ? 'roles' : 'clusterroles'

  return (
    <div className="card-inner">
      <div className="flex items-center gap-2 flex-wrap text-xs mb-1.5">
        <span className={clsx('badge', rbacKindBadgeClass(entry.binding.kind))}>
          {entry.binding.kind}
        </span>
        <ResourceLink
          kind={bindingKindPlural}
          namespace={entry.binding.namespace}
          name={entry.binding.name}
          onNavigate={onNavigate}
        />
        <span className="text-theme-text-secondary">→</span>
        <span className={clsx('badge', rbacKindBadgeClass(entry.binding.roleRef.kind))}>
          {entry.binding.roleRef.kind}
        </span>
        <ResourceLink
          kind={roleKindPlural}
          namespace={entry.binding.roleRef.namespace}
          name={entry.binding.roleRef.name}
          onNavigate={onNavigate}
        />
      </div>
      {entry.subjects.length > 0 && (
        <div className="text-xs text-theme-text-secondary">
          {entry.subjects.length} subject{entry.subjects.length === 1 ? '' : 's'}:{' '}
          <SubjectsInline subjects={entry.subjects} onNavigate={onNavigate} />
        </div>
      )}
    </div>
  )
}

function SubjectsInline({
  subjects,
  onNavigate,
}: {
  subjects: RBACSubject[]
  onNavigate?: (ref: ResourceRef) => void
}) {
  const shown = subjects.slice(0, 3)
  const hidden = subjects.length - shown.length
  return (
    <span className="inline-flex items-center gap-1.5 flex-wrap">
      {shown.map((s, i) => (
        <span key={i} className="inline-flex items-center gap-1">
          <span className="text-theme-text-tertiary">{s.kind.toLowerCase()}:</span>
          {s.kind === 'ServiceAccount' ? (
            <ResourceLink
              kind="serviceaccounts"
              namespace={s.namespace}
              name={s.name}
              onNavigate={onNavigate}
            />
          ) : (
            <span className="text-theme-text-primary">{s.name}</span>
          )}
          {i < shown.length - 1 && <span className="text-theme-text-tertiary">,</span>}
        </span>
      ))}
      {hidden > 0 && <span className="text-theme-text-tertiary">+{hidden} more</span>}
    </span>
  )
}
