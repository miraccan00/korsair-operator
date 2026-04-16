interface Props {
  message: string
  onRetry?: () => void
}

export function ErrorMessage({ message, onRetry }: Props) {
  return (
    <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 flex items-start gap-3">
      <span className="text-lg leading-none mt-0.5">⚠️</span>
      <div className="flex-1">
        <p className="font-medium">Error loading data</p>
        <p className="text-red-600 mt-0.5">{message}</p>
      </div>
      {onRetry && (
        <button
          onClick={onRetry}
          className="text-xs text-red-700 underline hover:text-red-900 whitespace-nowrap"
        >
          Retry
        </button>
      )}
    </div>
  )
}
