import { useCallback } from 'react'
import type { UIMessage } from 'ai'

/**
 * Serializes investigation messages to markdown for clipboard copy and file download.
 */
export function useExportInvestigation(messages: UIMessage[]) {
  const toMarkdown = useCallback(() => {
    const lines: string[] = []

    for (const msg of messages) {
      if (msg.role === 'user') {
        const text = msg.parts
          .filter((p): p is { type: 'text'; text: string } => p.type === 'text')
          .map(p => p.text)
          .join('')
        if (text) {
          lines.push(`**User:** ${text}`)
          lines.push('')
        }
        continue
      }

      // Assistant message
      for (const part of msg.parts) {
        if (part.type === 'text' && part.text) {
          lines.push(part.text)
          lines.push('')
        }

        if (part.type === 'dynamic-tool') {
          lines.push(`> **Tool:** ${part.toolName}`)
          if ('input' in part && part.input != null) {
            const inputStr = typeof part.input === 'string'
              ? part.input
              : JSON.stringify(part.input, null, 2)
            lines.push('> ```json')
            lines.push(`> ${inputStr.split('\n').join('\n> ')}`)
            lines.push('> ```')
          }
          lines.push('')
        }
      }
    }

    return lines.join('\n').trim()
  }, [messages])

  const copyToClipboard = useCallback(async () => {
    const md = toMarkdown()
    await navigator.clipboard.writeText(md)
  }, [toMarkdown])

  const downloadAsFile = useCallback(() => {
    const md = toMarkdown()
    const blob = new Blob([md], { type: 'text/markdown' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `investigation-${new Date().toISOString().slice(0, 10)}.md`
    a.click()
    URL.revokeObjectURL(url)
  }, [toMarkdown])

  return { copyToClipboard, downloadAsFile }
}
