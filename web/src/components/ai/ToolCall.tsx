import { useState, memo } from 'react'
import {
  ChevronRight,
  Box,
  Activity,
  ScrollText,
  GitCommit,
  Network,
  List,
  Loader2,
  CheckCircle2,
  AlertCircle,
} from 'lucide-react'
import { clsx } from 'clsx'
import type { DynamicToolUIPart } from 'ai'

const TOOL_ICONS: Record<string, typeof Box> = {
  get_resource: Box,
  get_events: Activity,
  get_pod_logs: ScrollText,
  get_changes: GitCommit,
  get_related_resources: Network,
  list_resources: List,
}

const TOOL_LABELS: Record<string, string> = {
  get_resource: 'Get Resource',
  get_events: 'Get Events',
  get_pod_logs: 'Get Pod Logs',
  get_changes: 'Get Changes',
  get_related_resources: 'Get Related Resources',
  list_resources: 'List Resources',
}

type ToolState = DynamicToolUIPart['state']

interface ToolCallProps {
  toolName: string
  toolCallId: string
  state: ToolState
  input?: unknown
  output?: unknown
  errorText?: string
}

export const ToolCall = memo(function ToolCall({
  toolName,
  state,
  input,
  output,
  errorText,
}: ToolCallProps) {
  const [expanded, setExpanded] = useState(false)

  const Icon = TOOL_ICONS[toolName] || Box
  const label = TOOL_LABELS[toolName] || toolName

  const isRunning = state === 'input-streaming' || state === 'input-available'
  const isComplete = state === 'output-available'
  const isError = state === 'output-error'

  const hasContent = (isComplete && output != null) || (input != null) || isError

  return (
    <div className="rounded-lg border border-theme-border/60 bg-theme-base/40 overflow-hidden">
      <button
        className={clsx(
          'w-full flex items-center gap-2.5 px-3 py-2 text-left transition-colors rounded-lg',
          hasContent && 'cursor-pointer hover:bg-theme-hover/30',
          !hasContent && 'cursor-default'
        )}
        onClick={() => hasContent && setExpanded(!expanded)}
      >
        {/* Expand chevron */}
        <ChevronRight
          className={clsx(
            'w-3 h-3 text-theme-text-tertiary transition-transform shrink-0',
            expanded && 'rotate-90',
            !hasContent && 'opacity-0'
          )}
        />

        {/* Tool icon */}
        <Icon className="w-3.5 h-3.5 text-purple-400 shrink-0" />

        {/* Tool name */}
        <span className="text-xs font-medium text-theme-text-secondary truncate flex-1">
          {label}
        </span>

        {/* Status badge */}
        {isRunning && (
          <Loader2 className="w-3.5 h-3.5 animate-spin text-purple-400 shrink-0" />
        )}
        {isComplete && (
          <CheckCircle2 className="w-3.5 h-3.5 text-green-500 shrink-0" />
        )}
        {isError && (
          <AlertCircle className="w-3.5 h-3.5 text-red-500 shrink-0" />
        )}
      </button>

      {/* Expandable content */}
      {expanded && hasContent && (
        <div className="px-3 pb-2.5 space-y-2">
          {/* Input args */}
          {input != null && (
            <div>
              <div className="text-[10px] uppercase tracking-wider text-theme-text-tertiary mb-1 font-medium">
                Input
              </div>
              <pre className="text-[11px] font-mono text-theme-text-tertiary bg-theme-base rounded-md p-2.5 overflow-x-auto whitespace-pre-wrap break-all max-h-40">
                {formatJSON(input)}
              </pre>
            </div>
          )}

          {/* Output */}
          {isComplete && output != null && (
            <div>
              <div className="text-[10px] uppercase tracking-wider text-theme-text-tertiary mb-1 font-medium">
                Output
              </div>
              <pre className="text-[11px] font-mono text-theme-text-tertiary bg-theme-base rounded-md p-2.5 overflow-x-auto whitespace-pre-wrap break-all max-h-60">
                {truncate(formatJSON(output), 3000)}
              </pre>
            </div>
          )}

          {/* Error */}
          {isError && errorText && (
            <div className="flex items-start gap-2 px-2.5 py-2 rounded-md bg-red-500/10 border border-red-500/20">
              <AlertCircle className="w-3.5 h-3.5 text-red-500 mt-0.5 shrink-0" />
              <span className="text-xs text-red-400 break-all">{errorText}</span>
            </div>
          )}
        </div>
      )}
    </div>
  )
})

function formatJSON(value: unknown): string {
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function truncate(s: string, max: number): string {
  if (s.length <= max) return s
  return s.slice(0, max) + '\n...(truncated)'
}
