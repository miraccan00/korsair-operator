import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import type { ImageScanJob, SummaryResponse } from '../api/types'
import { JobsTable } from '../components/JobsTable'
import { LoadingSpinner } from '../components/LoadingSpinner'
import { ErrorMessage } from '../components/ErrorMessage'
import { SummaryCard } from '../components/SummaryCard'
import { useAutoRefresh } from '../hooks/useAutoRefresh'

function formatTime(iso?: string): string {
  if (!iso) return 'Never'
  return new Date(iso).toLocaleString()
}

export function Dashboard() {
  const [summary, setSummary] = useState<SummaryResponse | null>(null)
  const [recentJobs, setRecentJobs] = useState<ImageScanJob[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastRefresh, setLastRefresh] = useState<Date>(new Date())

  const load = useCallback(async () => {
    try {
      const [s, jobs] = await Promise.all([api.getSummary(), api.getJobs()])
      setSummary(s)
      const sorted = [...jobs].sort(
        (a, b) =>
          new Date(b.metadata.creationTimestamp).getTime() -
          new Date(a.metadata.creationTimestamp).getTime()
      )
      setRecentJobs(sorted.slice(0, 10))
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
      setLastRefresh(new Date())
    }
  }, [])

  useEffect(() => { void load() }, [load])
  useAutoRefresh(load, 30_000)

  if (loading) return <LoadingSpinner message="Loading dashboard..." />
  if (error) return (
    <div className="p-6">
      <ErrorMessage message={error} onRetry={() => void load()} />
    </div>
  )

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Dashboard</h1>
        <p className="text-xs text-gray-400">
          Auto-refresh every 30s · Last updated {lastRefresh.toLocaleTimeString()}
        </p>
      </div>

      {/* Summary cards */}
      {summary && (
        <>
          <div>
            <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-3">
              Scan Activity
            </h2>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
              <SummaryCard label="Total Images"   value={summary.totalImages}   icon="🖼️" />
              <SummaryCard label="Active Jobs"    value={summary.activeJobs}    color="blue"  icon="⚙️" />
              <SummaryCard label="Completed"      value={summary.completedJobs} color="green" icon="✅" />
              <SummaryCard label="Failed"         value={summary.failedJobs}    color="red"   icon="❌" />
            </div>
          </div>

          <div>
            <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-3">
              Vulnerability Summary
            </h2>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
              <SummaryCard label="Critical"  value={summary.totalCritical} color="red"    icon="🔴" />
              <SummaryCard label="High"      value={summary.totalHigh}     color="orange" icon="🟠" />
              <SummaryCard label="Medium"    value={summary.totalMedium}   color="yellow" icon="🟡" />
              <SummaryCard label="Low"       value={summary.totalLow}                     icon="⚪" />
            </div>
          </div>

          {summary.lastScanTime && (
            <p className="text-sm text-gray-500">
              Last scan completed: <span className="font-medium">{formatTime(summary.lastScanTime)}</span>
            </p>
          )}
        </>
      )}

      {/* Recent jobs */}
      <div>
        <h2 className="text-lg font-semibold text-gray-800 mb-3">Recent Scan Jobs</h2>
        <JobsTable jobs={recentJobs} showConfig />
      </div>
    </div>
  )
}
