import type { ImageScanJobPhase } from '../api/types'

const phaseColors: Record<ImageScanJobPhase, string> = {
  Pending:   'bg-gray-100 text-gray-600 border border-gray-300',
  Running:   'bg-blue-100 text-blue-800 border border-blue-300',
  Completed: 'bg-green-100 text-green-800 border border-green-300',
  Failed:    'bg-red-100 text-red-800 border border-red-300',
}

const phaseIcons: Record<ImageScanJobPhase, string> = {
  Pending:   '⏳',
  Running:   '⚙️',
  Completed: '✅',
  Failed:    '❌',
}

export function PhaseBadge({ phase }: { phase?: ImageScanJobPhase }) {
  const p: ImageScanJobPhase = phase ?? 'Pending'
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold ${phaseColors[p]}`}>
      <span>{phaseIcons[p]}</span>
      {p}
    </span>
  )
}
