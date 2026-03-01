import { useState, useEffect, useRef, memo } from 'react'
import { createPortal } from 'react-dom'
import { X, Settings, Eye, EyeOff } from 'lucide-react'
import { clsx } from 'clsx'
import { useAIConfig, useUpdateAIConfig } from '../../api/client'
import { useAnimatedUnmount } from '../../hooks/useAnimatedUnmount'
import { TRANSITION_BACKDROP, TRANSITION_PANEL } from '../../utils/animation'

interface AISettingsDialogProps {
  open: boolean
  onClose: () => void
}

const PROVIDERS = [
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'ollama', label: 'Ollama / Custom' },
] as const

const PROVIDER_MODELS: Record<string, { value: string; label: string }[]> = {
  anthropic: [
    { value: 'claude-sonnet-4-6', label: 'Claude Sonnet 4.6' },
    { value: 'claude-opus-4-6', label: 'Claude Opus 4.6' },
    { value: 'claude-haiku-4-5-20251001', label: 'Claude Haiku 4.5 (fast, cheap)' },
  ],
  openai: [
    { value: 'gpt-5-mini', label: 'GPT-5 Mini' },
    { value: 'gpt-5.2', label: 'GPT-5.2' },
    { value: 'gpt-5', label: 'GPT-5' },
    { value: 'gpt-5-nano', label: 'GPT-5 Nano (fast, cheap)' },
  ],
  ollama: [
    { value: 'qwen3', label: 'Qwen 3' },
    { value: 'llama3.3', label: 'Llama 3.3' },
    { value: 'deepseek-r1', label: 'DeepSeek R1' },
    { value: 'mistral', label: 'Mistral' },
  ],
}

const PROVIDER_DEFAULTS: Record<string, { baseUrl: string; model: string }> = {
  anthropic: { baseUrl: '', model: 'claude-sonnet-4-6' },
  openai: { baseUrl: '', model: 'gpt-5-mini' },
  ollama: { baseUrl: 'http://localhost:11434/v1', model: 'qwen3' },
}

export const AISettingsDialog = memo(function AISettingsDialog({
  open,
  onClose,
}: AISettingsDialogProps) {
  const dialogRef = useRef<HTMLDivElement>(null)
  const { shouldRender, isOpen } = useAnimatedUnmount(open, 200)
  const { data: config } = useAIConfig()
  const updateConfig = useUpdateAIConfig()

  const [provider, setProvider] = useState('anthropic')
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [model, setModel] = useState('')
  const [customModel, setCustomModel] = useState(false)
  const [showKey, setShowKey] = useState(false)

  // Sync form state when config loads
  useEffect(() => {
    if (config?.configured) {
      const p = config.provider || 'anthropic'
      setProvider(p)
      setBaseUrl(config.baseUrl || '')
      const m = config.model || ''
      setModel(m)
      // Check if the saved model is in the known list
      const knownModels = PROVIDER_MODELS[p] || []
      setCustomModel(m !== '' && !knownModels.some(km => km.value === m))
    }
  }, [config])

  // Handle ESC key
  useEffect(() => {
    if (!open) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
      }
    }
    document.addEventListener('keydown', handleKeyDown, true)
    return () => document.removeEventListener('keydown', handleKeyDown, true)
  }, [open, onClose])

  // Focus trap
  useEffect(() => {
    if (open && dialogRef.current) {
      dialogRef.current.focus()
    }
  }, [open])

  const handleProviderChange = (newProvider: string) => {
    setProvider(newProvider)
    const defaults = PROVIDER_DEFAULTS[newProvider]
    if (defaults) {
      setBaseUrl(defaults.baseUrl)
      setModel(defaults.model)
      setCustomModel(false)
    }
  }

  const handleModelChange = (value: string) => {
    if (value === '__custom__') {
      setCustomModel(true)
      setModel('')
    } else {
      setCustomModel(false)
      setModel(value)
    }
  }

  const handleSave = () => {
    updateConfig.mutate(
      {
        provider,
        apiKey: apiKey || undefined,
        baseUrl: baseUrl || undefined,
        model: model || undefined,
      },
      { onSuccess: () => { setApiKey(''); onClose() } }
    )
  }

  const showBaseUrl = provider === 'openai' || provider === 'ollama'
  const showApiKey = provider !== 'ollama'
  const models = PROVIDER_MODELS[provider] || []

  if (!shouldRender) return null

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className={clsx(
          'absolute inset-0 bg-black/60 backdrop-blur-sm',
          TRANSITION_BACKDROP,
          isOpen ? 'opacity-100' : 'opacity-0'
        )}
        onClick={onClose}
      />

      {/* Dialog */}
      <div
        ref={dialogRef}
        tabIndex={-1}
        className={clsx(
          'relative bg-theme-surface border border-theme-border rounded-lg shadow-2xl max-w-md w-full mx-4 outline-none',
          TRANSITION_PANEL,
          isOpen ? 'opacity-100 scale-100' : 'opacity-0 scale-95'
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-theme-border">
          <div className="flex items-center gap-2.5">
            <Settings className="w-5 h-5 text-purple-400" />
            <h3 className="text-lg font-semibold text-theme-text-primary">AI Configuration</h3>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-theme-elevated rounded-md transition-colors cursor-pointer"
          >
            <X className="w-5 h-5 text-theme-text-tertiary" />
          </button>
        </div>

        {/* Form */}
        <div className="px-5 py-4 space-y-4">
          {/* Provider */}
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-theme-text-secondary">Provider</label>
            <select
              value={provider}
              onChange={(e) => handleProviderChange(e.target.value)}
              className="w-full bg-theme-base border border-theme-border rounded-md px-3 py-2 text-sm text-theme-text-primary outline-none focus:border-purple-500 transition-colors"
            >
              {PROVIDERS.map((p) => (
                <option key={p.value} value={p.value}>{p.label}</option>
              ))}
            </select>
          </div>

          {/* API Key */}
          {showApiKey && (
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-theme-text-secondary">API Key</label>
              <div className="relative">
                <input
                  type={showKey ? 'text' : 'password'}
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder={config?.configured ? '(unchanged)' : 'sk-...'}
                  className="w-full bg-theme-base border border-theme-border rounded-md px-3 py-2 pr-10 text-sm text-theme-text-primary outline-none focus:border-purple-500 transition-colors font-mono"
                />
                <button
                  type="button"
                  onClick={() => setShowKey(!showKey)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-theme-text-tertiary hover:text-theme-text-secondary cursor-pointer"
                >
                  {showKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
              <p className="text-[11px] text-theme-text-tertiary">
                Stored locally in ~/.radar/settings.json. Never sent anywhere except the selected provider.
              </p>
            </div>
          )}

          {/* Base URL */}
          {showBaseUrl && (
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-theme-text-secondary">Base URL</label>
              <input
                type="text"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder={provider === 'ollama' ? 'http://localhost:11434/v1' : 'https://api.openai.com/v1'}
                className="w-full bg-theme-base border border-theme-border rounded-md px-3 py-2 text-sm text-theme-text-primary outline-none focus:border-purple-500 transition-colors font-mono"
              />
            </div>
          )}

          {/* Model */}
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-theme-text-secondary">Model</label>
            {!customModel ? (
              <select
                value={model}
                onChange={(e) => handleModelChange(e.target.value)}
                className="w-full bg-theme-base border border-theme-border rounded-md px-3 py-2 text-sm text-theme-text-primary outline-none focus:border-purple-500 transition-colors"
              >
                {models.map((m) => (
                  <option key={m.value} value={m.value}>{m.label}</option>
                ))}
                <option value="__custom__">Custom model...</option>
              </select>
            ) : (
              <div className="flex gap-2">
                <input
                  type="text"
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder="Model name"
                  autoFocus
                  className="flex-1 bg-theme-base border border-theme-border rounded-md px-3 py-2 text-sm text-theme-text-primary outline-none focus:border-purple-500 transition-colors font-mono"
                />
                <button
                  type="button"
                  onClick={() => {
                    setCustomModel(false)
                    setModel(PROVIDER_DEFAULTS[provider]?.model || models[0]?.value || '')
                  }}
                  className="px-2 py-1 text-xs text-theme-text-tertiary hover:text-theme-text-secondary hover:bg-theme-elevated rounded transition-colors cursor-pointer"
                  title="Back to list"
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 px-5 py-4 border-t border-theme-border">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm font-medium text-theme-text-secondary hover:text-theme-text-primary hover:bg-theme-elevated rounded-lg transition-colors cursor-pointer"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={updateConfig.isPending}
            className="px-4 py-2 text-sm font-medium rounded-lg bg-purple-600 hover:bg-purple-700 text-white transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-2"
          >
            {updateConfig.isPending && (
              <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
            )}
            Save
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
})
