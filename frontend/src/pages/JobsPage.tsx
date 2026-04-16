import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import type { ImageScanJob, ImageScanJobPhase } from '../api/types'
import { JobsTable } from '../components/JobsTable'
import { LoadingSpinner } from '../components/LoadingSpinner'
import { ErrorMessage } from '../components/ErrorMessage'
import { useAutoRefresh } from '../hooks/useAutoRefresh'

const ALL_PHASES: ImageScanJobPhase[] = ['Pending', 'Running', 'Completed', 'Failed']

export function JobsPage() {
  const [jobs, setJobs] = useState<ImageScanJob[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [phaseFilter, setPhaseFilter] = useState<ImageScanJobPhase | 'All'>('All')
  const [scannerFilter, setScannerFilter] = useState<'All' | 'trivy' | 'grype'>('All')
  const [nsFilter, setNsFilter] = useState<string>('All')
  const [clusterFilter, setClusterFilter] = useState<string>('All')

  const load = useCallback(async () => {
    try {
      const data = await api.getJobs()
      setJobs(data)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])
  useAutoRefresh(load, 30_000)

  const namespaces = [...new Set(jobs.map((j) => j.spec.sourceNamespace ?? j.metadata.namespace ?? 'unknown'))].sort()
  const clusters   = [...new Set(jobs.map((j) => j.metadata.labels?.['security.blacksyrius.com/cluster-target'] ?? 'local'))].sort()

  const filtered = jobs.filter((j) => {
    if (phaseFilter !== 'All' && j.status.phase !== phaseFilter) return false
    if (scannerFilter !== 'All' && j.spec.scanner !== scannerFilter) return false
    if (nsFilter !== 'All' && (j.spec.sourceNamespace ?? j.metadata.namespace ?? 'unknown') !== nsFilter) return false
    if (clusterFilter !== 'All' && (j.metadata.labels?.['security.blacksyrius.com/cluster-target'] ?? 'local') !== clusterFilter) return false
    return true
  })

  // Sort by creationTimestamp descending
  const sorted = [...filtered].sort(
    (a, b) =>
      new Date(b.metadata.creationTimestamp).getTime() -
      new Date(a.metadata.creationTimestamp).getTime()
  )

  if (loading) return <LoadingSpinner message="Loading scan jobs..." />
  if (error) return (
    <div className="p-6">
      <ErrorMessage message={error} onRetry={() => void load()} />
    </div>
  )

  return (
    <div className="p-6 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Scan Jobs</h1>
        <p className="text-sm text-gray-500">{sorted.length} / {jobs.length} jobs</p>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        {/* Phase filter */}
        <div className="flex items-center gap-2">
          <label className="text-xs text-gray-500 font-medium">Phase:</label>
          <select
            value={phaseFilter}
            onChange={(e) => setPhaseFilter(e.target.value as typeof phaseFilter)}
            className="text-sm border border-gray-300 rounded px-2 py-1 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="All">All</option>
            {ALL_PHASES.map((p) => (
              <option key={p} value={p}>{p}</option>
            ))}
          </select>
        </div>

        {/* Scanner filter */}
        <div className="flex items-center gap-2">
          <label className="text-xs text-gray-500 font-medium">Scanner:</label>
          <select
            value={scannerFilter}
            onChange={(e) => setScannerFilter(e.target.value as typeof scannerFilter)}
            className="text-sm border border-gray-300 rounded px-2 py-1 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="All">All</option>
            <option value="trivy">trivy</option>
            <option value="grype">grype</option>
          </select>
        </div>

        {/* Namespace filter */}
        <div className="flex items-center gap-2">
          <label className="text-xs text-gray-500 font-medium">Namespace:</label>
          <select
            value={nsFilter}
            onChange={(e) => setNsFilter(e.target.value)}
            className="text-sm border border-gray-300 rounded px-2 py-1 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono"
          >
            <option value="All">All</option>
            {namespaces.map((ns) => (
              <option key={ns} value={ns}>{ns}</option>
            ))}
          </select>
        </div>

        {/* Cluster filter */}
        <div className="flex items-center gap-2">
          <label className="text-xs text-gray-500 font-medium">Cluster:</label>
          <select
            value={clusterFilter}
            onChange={(e) => setClusterFilter(e.target.value)}
            className="text-sm border border-gray-300 rounded px-2 py-1 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono"
          >
            <option value="All">All</option>
            {clusters.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
        </div>

        <button
          onClick={() => void load()}
          className="text-xs text-blue-600 hover:text-blue-800 border border-blue-300 hover:border-blue-500 px-3 py-1 rounded transition-colors"
        >
          ↻ Refresh
        </button>
      </div>

      <JobsTable jobs={sorted} showConfig showNamespace showCluster />
    </div>
  )
}
