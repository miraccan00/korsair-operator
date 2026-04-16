import { Link } from 'react-router-dom'
import type { ImageScanJob } from '../api/types'
import { PhaseBadge } from './PhaseBadge'
import { SeverityBadge } from './SeverityBadge'

interface Props {
  jobs: ImageScanJob[]
  showConfig?: boolean
  showNamespace?: boolean
  showCluster?: boolean
}

function formatTime(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

function shortImage(image: string): string {
  // Remove registry prefix for display (e.g. docker.io/library/nginx:1.25 → nginx:1.25)
  const parts = image.split('/')
  return parts[parts.length - 1] ?? image
}

export function JobsTable({ jobs, showConfig = false, showNamespace = false, showCluster = false }: Props) {
  if (jobs.length === 0) {
    return (
      <div className="text-center py-12 text-gray-400 text-sm">
        No scan jobs found.
      </div>
    )
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 shadow-sm">
      <table className="min-w-full divide-y divide-gray-200 text-sm">
        <thead className="bg-gray-50">
          <tr>
            <th className="px-4 py-3 text-left font-medium text-gray-600">Image</th>
            <th className="px-4 py-3 text-left font-medium text-gray-600">Scanner</th>
            {showNamespace && (
              <th className="px-4 py-3 text-left font-medium text-gray-600">Namespace</th>
            )}
            {showCluster && (
              <th className="px-4 py-3 text-left font-medium text-gray-600">Cluster</th>
            )}
            <th className="px-4 py-3 text-left font-medium text-gray-600">Status</th>
            <th className="px-4 py-3 text-left font-medium text-gray-600">CRIT</th>
            <th className="px-4 py-3 text-left font-medium text-gray-600">HIGH</th>
            <th className="px-4 py-3 text-left font-medium text-gray-600">MED</th>
            <th className="px-4 py-3 text-left font-medium text-gray-600">LOW</th>
            {showConfig && (
              <th className="px-4 py-3 text-left font-medium text-gray-600">Config</th>
            )}
            <th className="px-4 py-3 text-left font-medium text-gray-600">Completed</th>
            <th className="px-4 py-3 text-left font-medium text-gray-600">Report</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100 bg-white">
          {jobs.map((job) => {
            // jobNs: actual K8s namespace of the job resource (used for routing/API calls)
            const jobNs = job.metadata.namespace ?? 'bso-system'
            // displayNs: namespace where the scanned pod was found (used for display)
            const displayNs = job.spec.sourceNamespace ?? jobNs
            const name = job.metadata.name
            const cluster = job.metadata.labels?.['security.blacksyrius.com/cluster-target'] ?? 'local'
            const crit = job.status.criticalCount ?? 0
            const high = job.status.highCount ?? 0
            const med = job.status.mediumCount ?? 0
            const low = job.status.lowCount ?? 0

            return (
              <tr key={`${jobNs}/${name}`} className="hover:bg-gray-50 transition-colors">
                <td className="px-4 py-3">
                  <Link
                    to={`/jobs/${jobNs}/${name}`}
                    className="text-blue-600 hover:text-blue-800 hover:underline font-mono text-xs"
                    title={job.spec.image}
                  >
                    {shortImage(job.spec.image)}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 border border-purple-200">
                    {job.spec.scanner}
                  </span>
                </td>
                {showNamespace && (
                  <td className="px-4 py-3">
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-700 border border-gray-200 font-mono">
                      {displayNs}
                    </span>
                  </td>
                )}
                {showCluster && (
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border font-mono ${
                      cluster === 'local'
                        ? 'bg-blue-50 text-blue-700 border-blue-200'
                        : 'bg-emerald-50 text-emerald-700 border-emerald-200'
                    }`}>
                      {cluster}
                    </span>
                  </td>
                )}
                <td className="px-4 py-3">
                  <PhaseBadge phase={job.status.phase} />
                </td>
                <td className="px-4 py-3">
                  {crit > 0 ? <SeverityBadge severity="CRITICAL" count={crit} /> : <span className="text-gray-300">—</span>}
                </td>
                <td className="px-4 py-3">
                  {high > 0 ? <SeverityBadge severity="HIGH" count={high} /> : <span className="text-gray-300">—</span>}
                </td>
                <td className="px-4 py-3">
                  {med > 0 ? <SeverityBadge severity="MEDIUM" count={med} /> : <span className="text-gray-300">—</span>}
                </td>
                <td className="px-4 py-3">
                  {low > 0 ? <SeverityBadge severity="LOW" count={low} /> : <span className="text-gray-300">—</span>}
                </td>
                {showConfig && (
                  <td className="px-4 py-3 text-gray-500 text-xs">{job.spec.configRef}</td>
                )}
                <td className="px-4 py-3 text-gray-500 text-xs">
                  {formatTime(job.status.completionTime)}
                </td>
                <td className="px-4 py-3">
                  {job.status.phase === 'Completed' ? (
                    <Link
                      to={`/jobs/${jobNs}/${name}`}
                      className="text-xs text-blue-600 hover:underline"
                    >
                      View →
                    </Link>
                  ) : (
                    <span className="text-gray-300">—</span>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
