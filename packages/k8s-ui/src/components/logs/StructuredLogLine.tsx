import { useState, useMemo } from 'react'
import { ChevronRight, ChevronDown, Filter } from 'lucide-react'
import type { LogLevel } from './useLogBuffer'
import {
  getLevelColor,
  unescapeJsonStrings,
  parseLogfmt,
  SYNTAX_COLOR_KEY,
  SYNTAX_COLOR_STRING,
  SYNTAX_COLOR_NUMBER,
  SYNTAX_COLOR_BOOLEAN,
  SYNTAX_COLOR_NULL,
} from '../../utils/log-format'
import { SEVERITY_BADGE_BORDERED } from '../../utils/badge-colors'

interface StructuredLogLineProps {
  content: string
  level: LogLevel
  wordWrap: boolean
  isLogfmt?: boolean
  defaultExpanded?: boolean
  /**
   * When provided, field values in the expanded view become filterable — hovering
   * shows a chip that, on click, calls onFilterValue(value) so the parent can
   * add the value to the log search/filter.
   */
  onFilterValue?: (value: string) => void
}

export function StructuredLogLine({ content, level, wordWrap, isLogfmt, defaultExpanded, onFilterValue }: StructuredLogLineProps) {
  // null = user hasn't toggled this line; defers to defaultExpanded (global toggle)
  const [localExpanded, setLocalExpanded] = useState<boolean | null>(null)
  const expanded = localExpanded ?? defaultExpanded ?? false

  const parsed = useMemo(() => {
    try {
      if (isLogfmt) {
        return parseLogfmt(content)
      }
      return JSON.parse(content.trim())
    } catch {
      return null
    }
  }, [content, isLogfmt])

  if (!parsed) {
    return (
      <span className={`${wordWrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'} ${getLevelColor(level)}`}>
        {content}
      </span>
    )
  }

  const fieldCount = Object.keys(parsed).length

  const toggle = () => setLocalExpanded(!expanded)
  const chevron = expanded
    ? <ChevronDown className="w-3 h-3 shrink-0 text-theme-text-tertiary" />
    : <ChevronRight className="w-3 h-3 shrink-0 text-theme-text-tertiary" />

  return (
    <span>
      {!expanded ? (
        // Collapsed: entire summary line is clickable
        <span
          onClick={toggle}
          className={`cursor-pointer hover:bg-theme-surface/50 rounded px-0.5 -ml-0.5 ${wordWrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'}`}
        >
          <span className="inline-flex items-center align-middle mr-0.5">{chevron}</span>
          <SummaryLine obj={parsed} />
          <span className="text-theme-text-tertiary ml-1">{`{${fieldCount} fields}`}</span>
        </span>
      ) : (
        // Expanded: summary header is clickable to collapse, JSON content is selectable
        <>
        <span
          onClick={toggle}
          className="cursor-pointer hover:bg-theme-surface/50 rounded px-0.5 -ml-0.5"
        >
          <span className="inline-flex items-center align-middle mr-0.5">{chevron}</span>
          <SummaryLine obj={parsed} />
          <span className="text-theme-text-tertiary ml-1">{`{${fieldCount} fields}`}</span>
        </span>
        <span className={`block ml-4 ${wordWrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'}`}>
          {isLogfmt ? (
            <ExpandedLogfmt obj={parsed} onFilterValue={onFilterValue} />
          ) : (
            <JsonExpanded
              text={unescapeJsonStrings(JSON.stringify(parsed, null, 2))}
              onFilterValue={onFilterValue}
            />
          )}
        </span>
        </>
      )}
    </span>
  )
}

/**
 * Render a primitive log-field value with an optional filter chip that appears on hover.
 * Clicking the chip calls onFilter(value) so the caller can push it into log search.
 */
function FilterableValue({
  value, onFilter, color,
}: { value: string; onFilter?: (v: string) => void; color?: string }) {
  if (!onFilter) {
    return <span style={color ? { color } : undefined}>{value}</span>
  }
  return (
    <span className="group/flt inline-flex items-baseline align-baseline gap-0.5 rounded hover:bg-theme-surface/60">
      <span style={color ? { color } : undefined}>{value}</span>
      <button
        type="button"
        onClick={(e) => { e.stopPropagation(); onFilter(value) }}
        className="opacity-0 group-hover/flt:opacity-100 transition-opacity text-theme-text-tertiary hover:text-theme-text-primary px-0.5"
        title={`Filter to lines containing "${value}"`}
        aria-label={`Filter to lines containing ${value}`}
      >
        <Filter className="w-3 h-3 inline" />
      </button>
    </span>
  )
}

/**
 * Render pretty-printed JSON with filterable primitive values while preserving
 * the layout that JSON.stringify(..., null, 2) produces. Tokenizes the string
 * and emits React nodes so the hover chip can be wired per value.
 */
function JsonExpanded({ text, onFilterValue }: { text: string; onFilterValue?: (v: string) => void }) {
  const tokenRe = /("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|(-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b)|\b(true|false)\b|\b(null)\b/g
  const nodes: React.ReactNode[] = []
  let lastIndex = 0
  let match: RegExpExecArray | null
  let idx = 0
  while ((match = tokenRe.exec(text)) !== null) {
    if (match.index > lastIndex) {
      nodes.push(<span key={`t${idx++}`}>{text.slice(lastIndex, match.index)}</span>)
    }
    const [, key, str, num, bool, nil] = match
    if (key !== undefined) {
      nodes.push(<span key={`k${idx++}`} style={{ color: SYNTAX_COLOR_KEY }}>{key}</span>)
      nodes.push(<span key={`c${idx++}`}>:</span>)
    } else if (str !== undefined) {
      // Unescape the quoted string for the filter value (users expect to filter on
      // the displayed string, not JSON-escaped bytes).
      const inner = str.slice(1, -1).replace(/\\"/g, '"').replace(/\\\\/g, '\\')
      nodes.push(
        <FilterableValue key={`s${idx++}`} value={inner} onFilter={onFilterValue} color={SYNTAX_COLOR_STRING} />
      )
    } else if (num !== undefined) {
      nodes.push(
        <FilterableValue key={`n${idx++}`} value={num} onFilter={onFilterValue} color={SYNTAX_COLOR_NUMBER} />
      )
    } else if (bool !== undefined) {
      nodes.push(
        <FilterableValue key={`b${idx++}`} value={bool} onFilter={onFilterValue} color={SYNTAX_COLOR_BOOLEAN} />
      )
    } else if (nil !== undefined) {
      nodes.push(<span key={`z${idx++}`} style={{ color: SYNTAX_COLOR_NULL }}>{nil}</span>)
    }
    lastIndex = tokenRe.lastIndex
  }
  if (lastIndex < text.length) {
    nodes.push(<span key={`t${idx++}`}>{text.slice(lastIndex)}</span>)
  }
  return <>{nodes}</>
}

function SummaryLine({ obj }: { obj: Record<string, unknown> }) {
  const lvl = obj.level ?? obj.severity ?? obj.lvl ?? nestedField(obj, 'log', 'level')
  const msg = obj.msg ?? obj.message
  const rawErr = obj.error ?? obj.err
  const err = typeof rawErr === 'string'
    ? rawErr
    : nestedField(obj, 'error', 'message') ?? nestedField(obj, 'err', 'message')
  const caller = obj.caller ?? obj.source

  return (
    <>
      {lvl != null && (
        <span className={`${getLevelBadgeColor(lvl)} text-[10px] font-semibold px-1 py-px rounded mr-1.5 inline-block`}>
          {formatLevel(lvl)}
        </span>
      )}
      {typeof msg === 'string' && (
        <span className="text-theme-text-primary">{msg}</span>
      )}
      {typeof err === 'string' && (
        <span className="text-red-400 ml-2">error={err}</span>
      )}
      {typeof caller === 'string' && (
        <span className="text-theme-text-disabled ml-2">{caller}</span>
      )}
    </>
  )
}

function ExpandedLogfmt({ obj, onFilterValue }: { obj: Record<string, unknown>; onFilterValue?: (v: string) => void }) {
  return (
    <>
      {Object.entries(obj).map(([key, val]) => {
        const str = String(val)
        return (
          <div key={key}>
            <span style={{ color: SYNTAX_COLOR_KEY }}>{key}</span>
            <span className="text-theme-text-tertiary">=</span>
            <FilterableValue value={str} onFilter={onFilterValue} color={SYNTAX_COLOR_STRING} />
          </div>
        )
      })}
    </>
  )
}

function nestedField(obj: Record<string, unknown>, parent: string, child: string): unknown {
  const p = obj[parent]
  if (p && typeof p === 'object' && !Array.isArray(p)) {
    return (p as Record<string, unknown>)[child]
  }
  return undefined
}

function formatLevel(lvl: unknown): string {
  if (typeof lvl === 'number') {
    if (lvl >= 50) return 'ERR'
    if (lvl >= 40) return 'WARN'
    if (lvl >= 30) return 'INFO'
    return 'DBG'
  }
  return String(lvl).toUpperCase()
}

function getLevelBadgeColor(lvl: unknown): string {
  let normalized: string
  if (typeof lvl === 'number') {
    // Pino/bunyan numeric levels: 10=trace, 20=debug, 30=info, 40=warn, 50=error, 60=fatal
    if (lvl >= 50) normalized = 'error'
    else if (lvl >= 40) normalized = 'warn'
    else if (lvl >= 30) normalized = 'info'
    else normalized = 'debug'
  } else {
    normalized = String(lvl).toLowerCase()
  }
  if (/^(error|err|fatal|panic|critical|crit)$/.test(normalized)) return SEVERITY_BADGE_BORDERED.error
  if (/^(warn|warning)$/.test(normalized)) return SEVERITY_BADGE_BORDERED.warning
  if (/^(info|information|notice)$/.test(normalized)) return SEVERITY_BADGE_BORDERED.info
  if (/^(debug|dbg|trace|verbose)$/.test(normalized)) return SEVERITY_BADGE_BORDERED.debug
  return SEVERITY_BADGE_BORDERED.neutral
}
