import { describe, expect, it } from 'vitest'
import { renderToString } from 'react-dom/server'
import { ResourceRendererDispatch, getResourceStatus, type RendererOverrides } from './ResourceRendererDispatch'
import type { ResourceRef } from '../../types'

function renderWithScalers(scalers: ResourceRef[]): string {
  const overrides: RendererOverrides = {
    WorkloadRenderer: ({ scaleBlockedBy }) => (
      <span>{scaleBlockedBy?.map((ref) => ref.kind).join(',') || 'none'}</span>
    ),
  }

  return renderToString(
    <ResourceRendererDispatch
      resource={{ kind: 'deployments', namespace: 'prod', name: 'api' }}
      data={{ metadata: { name: 'api', namespace: 'prod' } }}
      relationships={{ scalers }}
      onCopy={() => {}}
      copied={null}
      rendererOverrides={overrides}
      showCommonSections={false}
    />,
  )
}

describe('ResourceRendererDispatch', () => {
  it('blocks manual replica scaling for HPA and KEDA ScaledObject scalers only', () => {
    const html = renderWithScalers([
      { kind: 'VerticalPodAutoscaler', namespace: 'prod', name: 'api-vpa' },
      { kind: 'HorizontalPodAutoscaler', namespace: 'prod', name: 'api-hpa' },
      { kind: 'ScaledObject', namespace: 'prod', name: 'api-keda' },
    ])

    expect(html).toContain('HorizontalPodAutoscaler,ScaledObject')
    expect(html).not.toContain('VerticalPodAutoscaler')
  })

  it('does not block manual replica scaling for VPA-only relationships', () => {
    const html = renderWithScalers([
      { kind: 'VerticalPodAutoscaler', namespace: 'prod', name: 'api-vpa' },
    ])

    expect(html).toContain('none')
  })
})

function renderKind(kind: string, data: any, namespace = ''): string {
  return renderToString(
    <ResourceRendererDispatch
      resource={{ kind, namespace, name: data?.metadata?.name || 'x' }}
      data={data}
      onCopy={() => {}}
      copied={null}
      showCommonSections={false}
    />,
  )
}

describe('ClusterPolicy kind collision (nvidia.com vs kyverno.io)', () => {
  it('routes nvidia.com ClusterPolicy to the GPU Operator renderer', () => {
    const html = renderKind('clusterpolicies', {
      apiVersion: 'nvidia.com/v1',
      kind: 'ClusterPolicy',
      metadata: { name: 'cluster-policy' },
      spec: { driver: { enabled: true }, devicePlugin: { enabled: true } },
      status: { state: 'ready' },
    })
    expect(html).toContain('Operator Status')
    expect(html).not.toContain('Rules')
  })

  it('routes kyverno.io ClusterPolicy to the Kyverno renderer', () => {
    const html = renderKind('clusterpolicies', {
      apiVersion: 'kyverno.io/v1',
      kind: 'ClusterPolicy',
      metadata: { name: 'require-labels' },
      spec: { rules: [{ name: 'check-labels' }] },
    })
    expect(html).not.toContain('Operator Status')
  })
})

describe('DRA renderers dispatch', () => {
  it('renders ResourceClaim with allocation sections (not GenericRenderer)', () => {
    const html = renderKind('resourceclaims', {
      apiVersion: 'resource.k8s.io/v1',
      kind: 'ResourceClaim',
      metadata: { name: 'gpu-claim', namespace: 'ml' },
      spec: { devices: { requests: [{ name: 'gpu', exactly: { deviceClassName: 'gpu.nvidia.com', count: 1 } }] } },
      status: {
        allocation: { devices: { results: [{ request: 'gpu', driver: 'gpu.nvidia.com', pool: 'node-1', device: 'gpu-0' }] } },
        reservedFor: [{ resource: 'pods', name: 'train-1' }],
      },
    }, 'ml')
    expect(html).toContain('Device Requests')
    expect(html).toContain('gpu.nvidia.com')
    expect(html).toContain('Reserved For')
  })

  it('does not treat an empty allocation block as allocated', () => {
    const html = renderKind('resourceclaims', {
      apiVersion: 'resource.k8s.io/v1',
      kind: 'ResourceClaim',
      metadata: { name: 'partial-claim', namespace: 'ml' },
      spec: { devices: { requests: [{ name: 'gpu', exactly: { deviceClassName: 'gpu.example.com' } }] } },
      status: { allocation: {} },
    }, 'ml')
    expect(html).toContain('Not allocated')
    expect(html).not.toContain('Allocated but unreserved')
  })

  it('reads v1beta1 request shape (deviceClassName at request level)', () => {
    const html = renderKind('resourceclaims', {
      apiVersion: 'resource.k8s.io/v1beta1',
      kind: 'ResourceClaim',
      metadata: { name: 'old-claim', namespace: 'ml' },
      spec: { devices: { requests: [{ name: 'gpu', deviceClassName: 'gpu.example.com' }] } },
    }, 'ml')
    expect(html).toContain('gpu.example.com')
  })

  it('renders ResourceSlice with device inventory', () => {
    const html = renderKind('resourceslices', {
      apiVersion: 'resource.k8s.io/v1',
      kind: 'ResourceSlice',
      metadata: { name: 'node-1-gpus' },
      spec: {
        driver: 'gpu.nvidia.com',
        pool: { name: 'node-1' },
        nodeName: 'node-1',
        devices: [{ name: 'gpu-0', attributes: { productName: { string: 'H100' } } }],
      },
    })
    expect(html).toContain('Slice Info')
    expect(html).toContain('H100')
  })

  it('renders DeviceClass selectors', () => {
    const html = renderKind('deviceclasses', {
      apiVersion: 'resource.k8s.io/v1',
      kind: 'DeviceClass',
      metadata: { name: 'gpu.nvidia.com' },
      spec: { selectors: [{ cel: { expression: "device.driver == 'gpu.nvidia.com'" } }] },
    })
    expect(html).toContain('Selectors (1)')
    expect(html).toContain('device.driver')
  })
})

describe('GPU ecosystem kind collisions', () => {
  it('routes Volcano Jobs away from the core JobRenderer to GenericRenderer', () => {
    const html = renderKind('jobs', {
      apiVersion: 'batch.volcano.sh/v1alpha1',
      kind: 'Job',
      metadata: { name: 'train', namespace: 'ml' },
      spec: { queue: 'gpu-queue', minAvailable: 4 },
      status: { state: { phase: 'Running' } },
    }, 'ml')
    expect(html).toContain('Min Available')
    expect(html).toContain('gpu-queue')
    expect(html).not.toContain('Completions')
  })

  it('routes status for queues by group (Volcano vs KAI)', () => {
    const volcano = getResourceStatus('queues', {
      apiVersion: 'scheduling.volcano.sh/v1beta1',
      status: { state: 'Open' },
    })
    const kai = getResourceStatus('queues', {
      apiVersion: 'scheduling.run.ai/v2',
      spec: { priority: 100 },
    })
    expect(volcano?.text).toBe('Open')
    expect(kai?.text).not.toBe('Open')
  })

  it('routes status for podgroups by group (Volcano vs KAI)', () => {
    const volcano = getResourceStatus('podgroups', {
      apiVersion: 'scheduling.volcano.sh/v1beta1',
      status: { phase: 'Running' },
    })
    expect(volcano?.text).toBe('Running')
  })

  it('routes Kueue Workload status only for the kueue group', () => {
    const admitted = getResourceStatus('workloads', {
      apiVersion: 'kueue.x-k8s.io/v1beta2',
      status: { conditions: [{ type: 'Admitted', status: 'True' }] },
    })
    expect(admitted?.text).toBe('Admitted')
  })

  it('routes KAITO Workspace status only for the kaito group', () => {
    const ws = getResourceStatus('workspaces', {
      apiVersion: 'kaito.sh/v1beta1',
      status: { conditions: [{ type: 'ResourceReady', status: 'False' }] },
    })
    expect(ws).not.toBeNull()
  })
})

describe('GPU ecosystem status edge cases', () => {
  it('JobSet with minimal status is Pending, not Running', () => {
    const fresh = getResourceStatus('jobsets', {
      apiVersion: 'jobset.x-k8s.io/v1alpha2',
      status: { replicatedJobsStatus: [{ name: 'w', active: 0, ready: 0 }] },
    })
    const live = getResourceStatus('jobsets', {
      apiVersion: 'jobset.x-k8s.io/v1alpha2',
      status: { replicatedJobsStatus: [{ name: 'w', active: 1, ready: 1 }] },
    })
    expect(fresh?.text).toBe('Pending')
    expect(live?.text).toBe('Running')
  })

  it('InferencePool with only an empty-parentRef default entry reads Not referenced', () => {
    const pool = getResourceStatus('inferencepools', {
      apiVersion: 'inference.networking.x-k8s.io/v1alpha2',
      status: { parent: [{ parentRef: {}, conditions: [{ type: 'Accepted', status: 'Unknown' }] }] },
    })
    expect(pool?.text).toBe('Not referenced')
  })
})
