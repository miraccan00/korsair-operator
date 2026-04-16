import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { ClusterTarget } from '../api/types'
import { ErrorMessage } from '../components/ErrorMessage'
import { LoadingSpinner } from '../components/LoadingSpinner'

function PhaseBadge({ phase }: { phase: string }) {
  if (phase === 'Connected')
    return <span className="px-2 py-0.5 rounded text-xs font-semibold bg-green-100 text-green-800">Connected</span>
  if (phase === 'Error')
    return <span className="px-2 py-0.5 rounded text-xs font-semibold bg-red-100 text-red-800">Error</span>
  return <span className="px-2 py-0.5 rounded text-xs font-semibold bg-gray-100 text-gray-600">Pending</span>
}

function timeAgo(iso: string) {
  const ms = Date.now() - new Date(iso).getTime()
  const m = Math.floor(ms / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

export function ClustersPage() {
  const [clusters, setClusters] = useState<ClusterTarget[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Form state
  const [displayName, setDisplayName] = useState('')
  const [kubeconfigFile, setKubeconfigFile] = useState<File | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const fetchClusters = () => {
    api.getClusters()
      .then(data => { setClusters(data); setError(null) })
      .catch(err => setError(String(err)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchClusters()
    const id = setInterval(fetchClusters, 30_000)
    return () => clearInterval(id)
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!kubeconfigFile) { setFormError('Please select a kubeconfig file.'); return }
    if (!displayName.trim()) { setFormError('Display name is required.'); return }
    setFormError(null)
    setSubmitting(true)
    try {
      await api.createCluster(displayName.trim(), kubeconfigFile)
      setDisplayName('')
      setKubeconfigFile(null)
      if (fileInputRef.current) fileInputRef.current.value = ''
      fetchClusters()
    } catch (err) {
      setFormError(String(err))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (!confirm(`Remove cluster "${name}"? This will delete its kubeconfig Secret.`)) return
    try {
      await api.deleteCluster(name)
      fetchClusters()
    } catch (err) {
      setError(String(err))
    }
  }

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Registered Clusters</h1>
        <p className="text-sm text-gray-500 mt-1">
          Add remote Kubernetes clusters to include in the scan pipeline. A read-only kubeconfig is sufficient.
        </p>
      </div>

      {/* Register form */}
      <div className="bg-white rounded-xl border border-gray-200 p-5">
        <h2 className="text-sm font-semibold text-gray-700 mb-4">Register New Cluster</h2>
        <form onSubmit={handleSubmit} className="flex flex-col sm:flex-row gap-3 items-end">
          <div className="flex-1">
            <label className="block text-xs text-gray-500 mb-1">Display Name</label>
            <input
              type="text"
              placeholder="e.g. Production EU"
              value={displayName}
              onChange={e => setDisplayName(e.target.value)}
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
          <div className="flex-1">
            <label className="block text-xs text-gray-500 mb-1">Kubeconfig File</label>
            <input
              ref={fileInputRef}
              type="file"
              accept=".yaml,.yml,.conf,.kubeconfig"
              onChange={e => setKubeconfigFile(e.target.files?.[0] ?? null)}
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-2 rounded-lg bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
          >
            {submitting ? 'Registering…' : 'Register Cluster'}
          </button>
        </form>
        {formError && (
          <p className="mt-2 text-xs text-red-600">{formError}</p>
        )}
      </div>

      {/* Cluster table */}
      {loading && <LoadingSpinner />}
      {error && <ErrorMessage message={error} />}
      {!loading && !error && (
        <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
          {clusters.length === 0 ? (
            <div className="p-10 text-center text-sm text-gray-400">
              No clusters registered yet. Upload a kubeconfig above to get started.
            </div>
          ) : (
            <table className="min-w-full divide-y divide-gray-200 text-sm">
              <thead className="bg-gray-50">
                <tr>
                  {['Display Name', 'Name', 'Phase', 'Nodes', 'Age', 'Message', ''].map(h => (
                    <th key={h} className="px-4 py-3 text-left text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {clusters.map(ct => (
                  <tr key={ct.name} className="hover:bg-gray-50">
                    <td className="px-4 py-3 font-medium text-gray-900">{ct.displayName}</td>
                    <td className="px-4 py-3 font-mono text-gray-500 text-xs">{ct.name}</td>
                    <td className="px-4 py-3"><PhaseBadge phase={ct.phase} /></td>
                    <td className="px-4 py-3 text-gray-700">{ct.nodeCount || '—'}</td>
                    <td className="px-4 py-3 text-gray-400">{timeAgo(ct.createdAt)}</td>
                    <td className="px-4 py-3 text-gray-400 text-xs max-w-xs truncate" title={ct.message}>{ct.message || '—'}</td>
                    <td className="px-4 py-3">
                      <button
                        onClick={() => handleDelete(ct.name)}
                        className="text-xs text-red-500 hover:text-red-700 font-medium"
                      >
                        Remove
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  )
}
