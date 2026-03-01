import { useState, memo } from 'react'
import { Sparkles } from 'lucide-react'
import { useAIConfig } from '../../api/client'
import { InvestigationPanel } from './InvestigationPanel'
import { AISettingsDialog } from './AISettingsDialog'

interface InvestigateButtonProps {
  kind: string
  namespace: string
  name: string
  variant?: 'icon' | 'button'
  /** Called when AI is configured and button is clicked.
   *  If provided, no standalone panel is opened — caller handles the UI. */
  onInvestigate?: () => void
}

export const InvestigateButton = memo(function InvestigateButton({
  kind,
  namespace,
  name,
  variant = 'icon',
  onInvestigate,
}: InvestigateButtonProps) {
  const [open, setOpen] = useState(false)
  const [hasOpened, setHasOpened] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const { data: config } = useAIConfig()

  const configured = config?.configured ?? false

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (configured) {
      if (onInvestigate) {
        onInvestigate()
      } else {
        setOpen(true)
        setHasOpened(true)
      }
    } else {
      setSettingsOpen(true)
    }
  }

  return (
    <>
      {variant === 'icon' ? (
        <button
          onClick={handleClick}
          className="p-1 text-purple-400 hover:text-purple-300 hover:bg-purple-500/10 rounded transition-colors cursor-pointer"
          title={configured ? 'Investigate with AI' : 'Set up AI to investigate'}
        >
          <Sparkles className="w-3.5 h-3.5" />
        </button>
      ) : (
        <button
          onClick={handleClick}
          className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium text-purple-400 hover:text-purple-300 hover:bg-purple-500/10 rounded-md transition-colors cursor-pointer"
          title={configured ? 'Investigate with AI' : 'Set up AI to investigate'}
        >
          <Sparkles className="w-3.5 h-3.5" />
          Investigate
        </button>
      )}
      {hasOpened && !onInvestigate && (
        <InvestigationPanel
          kind={kind}
          namespace={namespace}
          name={name}
          isOpen={open}
          onClose={() => setOpen(false)}
        />
      )}
      {settingsOpen && (
        <AISettingsDialog open={settingsOpen} onClose={() => setSettingsOpen(false)} />
      )}
    </>
  )
})
