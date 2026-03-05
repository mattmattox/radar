import { memo } from 'react'
import { NodeProps, Handle, Position } from '@xyflow/react'
import { ChevronDown, ChevronRight, Box, Tag } from 'lucide-react'

interface GroupNodeData {
  type: 'namespace' | 'app' | 'label'
  name: string
  label?: string
  nodeCount: number
  collapsed: boolean
  onToggleCollapse: (groupId: string) => void
  hideHeader?: boolean
}

export const GroupNode = memo(function GroupNode({
  id,
  data,
  width,
  height,
}: NodeProps & { data: GroupNodeData }) {
  const { type, name, label, nodeCount, collapsed, onToggleCollapse, hideHeader } = data

  const getIcon = () => {
    switch (type) {
      case 'namespace':
        return Box
      case 'app':
      case 'label':
        return Tag
      default:
        return Box
    }
  }

  const getBorderStyle = (): React.CSSProperties => {
    // Must set full 'border' property to override ReactFlow's --xy-node-border
    switch (type) {
      case 'namespace':
        return { border: '2px solid var(--group-border-namespace)' }
      case 'app':
        return { border: '2px solid var(--group-border-app)' }
      case 'label':
        return { border: '2px solid var(--group-border-label)' }
      default:
        return { border: '2px solid var(--border-default)' }
    }
  }

  const getHeaderBgStyle = (): React.CSSProperties => {
    switch (type) {
      case 'namespace':
        return { backgroundColor: 'var(--group-header-namespace)' }
      case 'app':
        return { backgroundColor: 'var(--group-header-app)' }
      case 'label':
        return { backgroundColor: 'var(--group-header-label)' }
      default:
        return { backgroundColor: 'var(--bg-hover)' }
    }
  }

  const getLabelStyle = (): React.CSSProperties => {
    switch (type) {
      case 'namespace':
        return { color: 'var(--group-label-namespace)' }
      case 'app':
        return { color: 'var(--group-label-app)' }
      case 'label':
        return { color: 'var(--group-label-label)' }
      default:
        return { color: 'var(--text-secondary)' }
    }
  }

  const getIconStyle = (): React.CSSProperties => {
    switch (type) {
      case 'namespace':
        return { color: 'var(--group-icon-namespace)' }
      case 'app':
        return { color: 'var(--group-icon-app)' }
      case 'label':
        return { color: 'var(--group-icon-label)' }
      default:
        return { color: 'var(--text-secondary)' }
    }
  }

  const Icon = getIcon()

  // When collapsed, render as a compact card
  if (collapsed) {
    return (
      <>
        <Handle
          type="target"
          position={Position.Left}
          className="!bg-transparent !border-0 !w-0 !h-0"
        />

        <div
          className="rounded-xl p-4 cursor-pointer group-header-scaled"
          onClick={() => onToggleCollapse(id)}
          style={{ ...getBorderStyle(), ...getHeaderBgStyle() }}
        >
          <div className="flex items-center gap-4">
            <ChevronRight className="w-8 h-8" style={getIconStyle()} />
            <Icon className="w-9 h-9" style={getIconStyle()} />
            <span className="text-4xl font-bold" style={getLabelStyle()}>{name}</span>
            {label && (
              <span className="text-sm text-theme-text-secondary">({label})</span>
            )}
          </div>
          <div className="mt-3 text-xl text-theme-text-secondary">
            {nodeCount} {nodeCount === 1 ? 'resource' : 'resources'}
          </div>
        </div>

        <Handle
          type="source"
          position={Position.Right}
          className="!bg-transparent !border-0 !w-0 !h-0"
        />
      </>
    )
  }

  // When expanded, render as a container with header
  // Children are rendered automatically by ReactFlow via parentId
  return (
    <>
      <Handle
        type="target"
        position={Position.Left}
        className="!bg-transparent !border-0 !w-0 !h-0"
      />

      {/* Container with border - use explicit dimensions from props */}
      {/* Top position adjusts based on CSS variable set by ViewportController */}
      <div
        className="absolute left-0 rounded-xl box-border isolate overflow-hidden bg-theme-surface/40 group-container-adjusted"
        style={{
          width: width || '100%',
          height: height || '100%',
          ...getBorderStyle()
        }}
      >
        {/* Header bar - background extends full width, content scales, count fixed right */}
        {/* Hidden when hideHeader is true (single namespace view) */}
        {!hideHeader && (
          <div
            className="w-full cursor-pointer relative flex items-center"
            onClick={() => onToggleCollapse(id)}
            style={getHeaderBgStyle()}
          >
            {/* Scaled content */}
            <div
              className="flex items-center group-header-scaled"
              style={{
                padding: '20px 24px',
                gap: '16px',
              }}
            >
              <ChevronDown
                className="shrink-0 w-8 h-8"
                style={getIconStyle()}
              />
              <Icon
                className="shrink-0 w-9 h-9"
                style={getIconStyle()}
              />
              <span
                className="font-bold truncate text-4xl"
                style={getLabelStyle()}
              >
                {name}
              </span>
              {label && (
                <span className="text-sm text-theme-text-secondary truncate">
                  ({label})
                </span>
              )}
            </div>
            {/* Count badge - fixed size, anchored top-right */}
            <span className="absolute right-4 top-4 shrink-0 font-medium text-sm text-theme-text-secondary bg-theme-surface/70 rounded-lg px-3 py-1">
              {nodeCount}
            </span>
          </div>
        )}
      </div>

      <Handle
        type="source"
        position={Position.Right}
        className="!bg-transparent !border-0 !w-0 !h-0"
      />
    </>
  )
})
