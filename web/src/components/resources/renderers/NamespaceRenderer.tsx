import { NamespaceRenderer as BaseNamespaceRenderer } from '@skyhook-io/k8s-ui/components/resources/renderers/NamespaceRenderer'
import type { ResourceRef } from '@skyhook-io/k8s-ui'
import { useRBACNamespace } from '../../../api/rbac'

interface NamespaceRendererProps {
  data: any
  onNavigate?: (ref: ResourceRef) => void
}

export function NamespaceRenderer({ data, onNavigate }: NamespaceRendererProps) {
  const name = data?.metadata?.name ?? ''
  const { data: rbacData, isLoading, error } = useRBACNamespace(name, !!name)
  return (
    <BaseNamespaceRenderer
      data={data}
      rbacData={rbacData ?? null}
      rbacLoading={isLoading}
      rbacError={error as Error | null}
      onNavigate={onNavigate}
    />
  )
}
