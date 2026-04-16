interface Props {
  severity: string
  count?: number
}

const colorMap: Record<string, string> = {
  CRITICAL: 'bg-red-100 text-red-800 border border-red-300',
  HIGH:     'bg-orange-100 text-orange-800 border border-orange-300',
  MEDIUM:   'bg-yellow-100 text-yellow-800 border border-yellow-300',
  LOW:      'bg-gray-100 text-gray-600 border border-gray-300',
}

export function SeverityBadge({ severity, count }: Props) {
  const key = severity.toUpperCase()
  const cls = colorMap[key] ?? colorMap['LOW']
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold ${cls}`}>
      {severity}{count !== undefined ? `: ${count}` : ''}
    </span>
  )
}
