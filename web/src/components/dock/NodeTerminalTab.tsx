import { NodeTerminalTab as SharedNodeTerminalTab } from '@skyhook-io/k8s-ui'

interface NodeTerminalTabProps {
  nodeName: string
  isActive?: boolean
}

export function NodeTerminalTab({ nodeName, isActive }: NodeTerminalTabProps) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

  const createNodeDebugPod = async (name: string) => {
    const response = await fetch(`/api/nodes/${encodeURIComponent(name)}/debug`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    })
    if (!response.ok) {
      const err = await response.json().catch(() => ({ error: 'Unknown error' }))
      throw new Error(err.error || `HTTP ${response.status}`)
    }
    return response.json()
  }

  const cleanupNodeDebugPod = async (name: string) => {
    try {
      await fetch(`/api/nodes/${encodeURIComponent(name)}/debug`, {
        method: 'DELETE',
        keepalive: true,
      })
    } catch (err) {
      console.warn(`[NodeTerminal] Failed to cleanup debug pod for node ${name}:`, err)
    }
  }

  const createSession = async (namespace: string, podName: string, containerName: string) => ({
    wsUrl: `${protocol}//${window.location.host}/api/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(podName)}/exec?container=${encodeURIComponent(containerName)}`,
  })

  return (
    <SharedNodeTerminalTab
      nodeName={nodeName}
      isActive={isActive}
      createNodeDebugPod={createNodeDebugPod}
      cleanupNodeDebugPod={cleanupNodeDebugPod}
      createSession={createSession}
    />
  )
}
