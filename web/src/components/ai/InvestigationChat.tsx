import { useState, useEffect, useRef, useCallback, useMemo, memo } from 'react'
import {
  Sparkles,
  Settings,
  Square,
  Loader2,
  Send,
  Copy,
  Download,
  Check,
} from 'lucide-react'
import { useChat } from '@ai-sdk/react'
import { DefaultChatTransport } from 'ai'
import { ChatMessage } from './ChatMessage'
import { AISettingsDialog } from './AISettingsDialog'
import { useExportInvestigation } from './useExportInvestigation'
import type { UIMessage } from 'ai'

interface InvestigationChatProps {
  kind: string
  namespace: string
  name: string
}

export const InvestigationChat = memo(function InvestigationChat({
  kind,
  namespace,
  name,
}: InvestigationChatProps) {
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const autoTriggered = useRef(false)

  const { messages, sendMessage, status, stop } = useChat({
    transport: new DefaultChatTransport({
      api: '/api/ai/investigate',
      prepareSendMessagesRequest: ({ messages: msgs, body }) => ({
        body: {
          ...body,
          kind,
          namespace,
          name,
          ...(msgs.length > 1
            ? { question: msgs[msgs.length - 1]?.parts?.find(p => p.type === 'text')?.text }
            : {}),
        },
      }),
    }),
  })

  const { copyToClipboard, downloadAsFile } = useExportInvestigation(messages)

  const isStreaming = status === 'streaming' || status === 'submitted'
  const isComplete = status === 'ready' && messages.length > 0
  const hasAssistantMessages = messages.some(m => m.role === 'assistant')

  // Auto-scroll to bottom as new content arrives
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  // Auto-trigger investigation on mount
  useEffect(() => {
    if (autoTriggered.current) return
    autoTriggered.current = true
    sendMessage({ text: `Investigate ${kind} ${namespace ? namespace + '/' : ''}${name}` })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Focus input when investigation completes
  useEffect(() => {
    if (isComplete && inputRef.current) {
      inputRef.current.focus()
    }
  }, [isComplete])

  const [followUp, setFollowUp] = useState('')

  const handleFollowUp = useCallback(() => {
    if (!followUp.trim()) return
    sendMessage({ text: followUp.trim() })
    setFollowUp('')
  }, [followUp, sendMessage])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        handleFollowUp()
      }
    },
    [handleFollowUp]
  )

  const handleCopy = useCallback(async () => {
    await copyToClipboard()
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [copyToClipboard])

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-theme-border shrink-0">
        <div className="flex items-center gap-2.5 min-w-0">
          <Sparkles className="w-4 h-4 text-purple-400 shrink-0" />
          <h3 className="text-sm font-semibold text-theme-text-primary truncate">
            AI Investigation
          </h3>
        </div>
        <button
          onClick={() => setSettingsOpen(true)}
          className="p-1.5 hover:bg-theme-elevated rounded-md transition-colors cursor-pointer"
          title="AI settings"
        >
          <Settings className="w-4 h-4 text-theme-text-tertiary" />
        </button>
      </div>

      {/* Messages */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto min-h-0 px-4 py-3 space-y-4">
        {messages.map((msg) => (
          <ChatMessage key={msg.id} message={msg} />
        ))}
        {isStreaming && <StreamingStatus messages={messages} />}
      </div>

      {/* Action bar — export buttons when complete */}
      {isComplete && hasAssistantMessages && (
        <div className="px-4 py-2 border-t border-theme-border/50 flex items-center gap-2 shrink-0">
          <button
            onClick={handleCopy}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs text-theme-text-tertiary hover:text-theme-text-secondary hover:bg-theme-elevated rounded-md transition-colors cursor-pointer"
          >
            {copied ? (
              <Check className="w-3.5 h-3.5 text-green-500" />
            ) : (
              <Copy className="w-3.5 h-3.5" />
            )}
            {copied ? 'Copied' : 'Copy'}
          </button>
          <button
            onClick={downloadAsFile}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs text-theme-text-tertiary hover:text-theme-text-secondary hover:bg-theme-elevated rounded-md transition-colors cursor-pointer"
          >
            <Download className="w-3.5 h-3.5" />
            Export
          </button>
        </div>
      )}

      {/* Footer — input or stop button */}
      <div className="px-4 py-3 border-t border-theme-border shrink-0 space-y-2">
        {isStreaming ? (
          <button
            onClick={() => stop()}
            className="w-full flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium rounded-lg border border-theme-border hover:bg-theme-elevated transition-colors cursor-pointer text-theme-text-secondary"
          >
            <Square className="w-3.5 h-3.5" />
            Stop
          </button>
        ) : (
          <div className="flex items-center gap-2">
            <input
              ref={inputRef}
              type="text"
              value={followUp}
              onChange={(e) => setFollowUp(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask a follow-up question..."
              className="flex-1 bg-theme-base border border-theme-border rounded-md px-3 py-2 text-sm text-theme-text-primary outline-none focus:border-purple-500 transition-colors placeholder:text-theme-text-tertiary"
            />
            <button
              onClick={handleFollowUp}
              disabled={!followUp.trim()}
              className="p-2 rounded-md bg-purple-600 hover:bg-purple-700 text-white transition-colors disabled:opacity-30 cursor-pointer"
            >
              <Send className="w-4 h-4" />
            </button>
          </div>
        )}
      </div>

      {settingsOpen && (
        <AISettingsDialog open={settingsOpen} onClose={() => setSettingsOpen(false)} />
      )}
    </div>
  )
})

// -- Streaming status indicator -----------------------------------------------

const TOOL_LABELS: Record<string, string> = {
  get_resource: 'Get Resource',
  get_events: 'Get Events',
  get_pod_logs: 'Get Pod Logs',
  get_changes: 'Get Changes',
  get_related_resources: 'Get Related Resources',
  list_resources: 'List Resources',
}

function getStreamingStatus(messages: UIMessage[]): string | null {
  const lastMsg = [...messages].reverse().find(m => m.role === 'assistant')
  if (!lastMsg || lastMsg.parts.length === 0) return 'Starting investigation...'

  for (let i = lastMsg.parts.length - 1; i >= 0; i--) {
    const part = lastMsg.parts[i]

    if (part.type === 'text' && 'state' in part && part.state === 'streaming') {
      return null
    }

    if (part.type === 'dynamic-tool') {
      if ('state' in part && part.state !== 'output-available') {
        const label = TOOL_LABELS[part.toolName] ?? part.toolName
        return `Running ${label}...`
      }
      return 'Analyzing findings...'
    }

    if (typeof part.type === 'string' && part.type.startsWith('data-')) {
      const data = 'data' in part ? (part.data as Record<string, unknown>) : null
      if (data?.content) return String(data.content)
    }

    if (part.type === 'step-start') {
      return 'Thinking...'
    }
  }

  return 'Thinking...'
}

const StreamingStatus = memo(function StreamingStatus({ messages }: { messages: UIMessage[] }) {
  const status = useMemo(() => getStreamingStatus(messages), [messages])
  if (!status) return null

  return (
    <div className="flex items-center gap-2 py-1.5">
      <Loader2 className="w-3.5 h-3.5 animate-spin text-purple-400 shrink-0" />
      <span className="text-xs text-theme-text-tertiary">{status}</span>
    </div>
  )
})
