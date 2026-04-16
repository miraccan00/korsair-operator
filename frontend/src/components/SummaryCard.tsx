type Color = 'default' | 'blue' | 'green' | 'red' | 'orange' | 'yellow'

interface Props {
  label: string
  value: number
  color?: Color
  icon?: string
}

const colorMap: Record<Color, { card: string; value: string }> = {
  default: { card: 'bg-white border-gray-200',   value: 'text-gray-900' },
  blue:    { card: 'bg-blue-50 border-blue-200',  value: 'text-blue-700' },
  green:   { card: 'bg-green-50 border-green-200',value: 'text-green-700' },
  red:     { card: 'bg-red-50 border-red-200',    value: 'text-red-700' },
  orange:  { card: 'bg-orange-50 border-orange-200', value: 'text-orange-700' },
  yellow:  { card: 'bg-yellow-50 border-yellow-200', value: 'text-yellow-700' },
}

export function SummaryCard({ label, value, color = 'default', icon }: Props) {
  const { card, value: valueColor } = colorMap[color]
  return (
    <div className={`rounded-lg border p-4 shadow-sm ${card}`}>
      <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">
        {icon && <span className="mr-1">{icon}</span>}
        {label}
      </p>
      <p className={`mt-1 text-3xl font-bold ${valueColor}`}>{value}</p>
    </div>
  )
}
