import { memo } from 'react'
import { Markdown } from '../ui/Markdown'
import { ToolCall } from './ToolCall'
import type { UIMessage } from 'ai'

interface ChatMessageProps {
  message: UIMessage
}

export const ChatMessage = memo(function ChatMessage({
  message,
}: ChatMessageProps) {
  if (message.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] px-3 py-2 rounded-lg bg-purple-600/20 border border-purple-500/20">
          <p className="text-sm text-theme-text-primary">
            {message.parts.map((part, i) =>
              part.type === 'text' ? <span key={i}>{part.text}</span> : null
            )}
          </p>
        </div>
      </div>
    )
  }

  // Assistant message — render parts
  return (
    <div className="space-y-2">
      {message.parts.map((part, i) => {
        // Text parts
        if (part.type === 'text') {
          if (!part.text) return null
          return (
            <div key={i} className="relative">
              <Markdown className="text-sm">{part.text}</Markdown>
              {part.state === 'streaming' && (
                <span className="inline-block w-1.5 h-4 bg-purple-400 animate-pulse ml-0.5 -mb-0.5 rounded-sm" />
              )}
            </div>
          )
        }

        // Dynamic tool calls (our tools are untyped)
        if (part.type === 'dynamic-tool') {
          return (
            <ToolCall
              key={part.toolCallId}
              toolName={part.toolName}
              toolCallId={part.toolCallId}
              state={part.state}
              input={'input' in part ? part.input : undefined}
              output={'output' in part ? part.output : undefined}
              errorText={'errorText' in part ? part.errorText : undefined}
            />
          )
        }

        // Data parts (status events) — rendered by InvestigationPanel as a bottom indicator
        if (typeof part.type === 'string' && part.type.startsWith('data-')) {
          return null
        }

        // Step start parts
        if (part.type === 'step-start') {
          return null // Invisible, just marks step boundaries
        }

        return null
      })}

    </div>
  )
})

