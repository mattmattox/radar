import { memo } from 'react'
import { createPortal } from 'react-dom'
import { clsx } from 'clsx'
import { X } from 'lucide-react'
import { InvestigationChat } from './InvestigationChat'
import { useAnimatedUnmount } from '../../hooks/useAnimatedUnmount'
import { TRANSITION_BACKDROP, TRANSITION_DRAWER } from '../../utils/animation'

interface InvestigationPanelProps {
  kind: string
  namespace: string
  name: string
  isOpen: boolean
  onClose: () => void
}

/**
 * Standalone portal-based investigation panel.
 * Used from ResourceDetailPage, HomeView, and other non-drawer contexts.
 * For the drawer context, InvestigationChat is rendered inline instead.
 */
export const InvestigationPanel = memo(function InvestigationPanel({
  kind,
  namespace,
  name,
  isOpen,
  onClose,
}: InvestigationPanelProps) {
  const { shouldRender, isOpen: animIsOpen } = useAnimatedUnmount(isOpen, 300)

  if (!shouldRender) return null

  return createPortal(
    <>
      {/* Backdrop */}
      <div
        className={clsx(
          'fixed inset-0 bg-black/50 z-40',
          TRANSITION_BACKDROP,
          animIsOpen ? 'opacity-100' : 'opacity-0'
        )}
        onClick={onClose}
      />

      {/* Panel */}
      <div
        className={clsx(
          'fixed right-0 top-0 bottom-0 w-[520px] max-w-full bg-theme-surface border-l border-theme-border z-50 flex flex-col shadow-2xl',
          TRANSITION_DRAWER,
          animIsOpen ? 'translate-x-0 opacity-100' : 'translate-x-full opacity-0'
        )}
      >
        {/* Close button overlay in top-right corner */}
        <button
          onClick={onClose}
          className="absolute right-3 top-3 p-1.5 hover:bg-theme-elevated rounded-md transition-colors cursor-pointer z-10"
        >
          <X className="w-5 h-5 text-theme-text-tertiary" />
        </button>

        <InvestigationChat kind={kind} namespace={namespace} name={name} />
      </div>
    </>,
    document.body
  )
})
