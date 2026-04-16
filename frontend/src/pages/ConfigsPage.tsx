import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import type { SecurityScanConfig } from '../api/types'
import { LoadingSpinner } from '../components/LoadingSpinner'
import { ErrorMessage } from '../components/ErrorMessage'
import { useAutoRefresh } from '../hooks/useAutoRefresh'

function formatTime(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

function RelativeTime({ iso }: { iso?: string }) {
  if (!iso) return <span className="text-gray-400">—</span>
  const diff = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diff / 60000)
  const hrs = Math.floor(mins / 60)
  const days = Math.floor(hrs / 24)
  let label = ''
  if (days > 0) label = `${days}d ago`
  else if (hrs > 0) label = `${hrs}h ago`
  else if (mins > 0) label = `${mins}m ago`
  else label = 'just now'
  return <span title={formatTime(iso)}>{label}</span>
}

function Badge({ label, color = 'gray' }: { label: string; color?: 'gray' | 'purple' | 'blue' | 'green' | 'red' | 'yellow' }) {
  const cls: Record<string, string> = {
    gray:   'bg-gray-100 text-gray-600 border-gray-200',
    purple: 'bg-purple-100 text-purple-800 border-purple-200',
    blue:   'bg-blue-100 text-blue-800 border-blue-200',
    green:  'bg-green-100 text-green-800 border-green-200',
    red:    'bg-red-100 text-red-700 border-red-200',
    yellow: 'bg-yellow-100 text-yellow-800 border-yellow-200',
  }
  return (
    <span className={`inline-flex items-center text-xs px-2 py-0.5 rounded border font-medium ${cls[color]}`}>
      {label}
    </span>
  )
}

function StatusPill({ active, failed }: { active: boolean; failed: number }) {
  if (active) return <Badge label="Scanning" color="blue" />
  if (failed > 0) return <Badge label={`${failed} Failed`} color="red" />
  return <Badge label="Idle" color="green" />
}

function InfoIcon({ tip }: { tip: string }) {
  return (
    <span
      className="inline-flex items-center justify-center w-3.5 h-3.5 rounded-full bg-gray-200 text-gray-500 text-[9px] font-bold cursor-default ml-1 align-middle"
      title={tip}
    >
      i
    </span>
  )
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <p className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1.5">{children}</p>
}

export function ConfigsPage() {
  const [configs, setConfigs] = useState<SecurityScanConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await api.getConfigs()
      setConfigs(data)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])
  useAutoRefresh(load, 30_000)

  if (loading) return <LoadingSpinner message="Loading configs..." />
  if (error) return (
    <div className="p-6">
      <ErrorMessage message={error} onRetry={() => void load()} />
    </div>
  )

  return (
    <div className="p-6 space-y-5">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Scan Configs</h1>
        <span className="text-sm text-gray-400">{configs.length} SecurityScanConfig{configs.length !== 1 ? 's' : ''}</span>
      </div>

      {configs.length === 0 ? (
        <div className="text-center py-16 text-gray-400 text-sm">
          No SecurityScanConfig resources found.
        </div>
      ) : (
        <div className="space-y-5">
          {configs.map((cfg) => {
            const spec = cfg.spec
            const s = cfg.status
            const active = (s.activeJobs ?? 0) > 0
            const failed = s.failedJobs ?? 0
            const excludeNS = spec.imageSources?.kubernetes?.excludeNamespaces ?? []
            const clusterRefs = spec.imageSources?.clusterTargetRefs ?? []
            const discoverAll = spec.imageSources?.discoverAllClusters ?? false
            const scanners = spec.scanners ?? ['trivy']
            const targetNS = spec.targetNamespace ?? 'korsair-system'
            const readyCond = s.conditions?.find(c => c.type === 'Ready')

            return (
              <div key={cfg.metadata.name} className="rounded-xl border border-gray-200 bg-white shadow-sm overflow-hidden">

                {/* ── Card Header ───────────────────────────────────────── */}
                <div className="flex items-center justify-between px-5 py-4 border-b border-gray-100 bg-gray-50">
                  <div className="flex items-center gap-3 min-w-0">
                    <h2 className="text-base font-semibold text-gray-900 font-mono truncate">{cfg.metadata.name}</h2>
                    <StatusPill active={active} failed={failed} />
                    {spec.schedule && (
                      <Badge label={`⏱ ${spec.schedule}`} color="gray" />
                    )}
                  </div>
                  {readyCond && (
                    <span className={`text-xs font-medium ${readyCond.status === 'True' ? 'text-green-600' : 'text-red-500'}`}>
                      {readyCond.status === 'True' ? '● Ready' : '● Not Ready'}
                    </span>
                  )}
                </div>

                <div className="px-5 py-4 space-y-5">

                  {/* ── Spec Section ──────────────────────────────────────── */}
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-4">

                    {/* Scanners */}
                    <div>
                      <SectionLabel>Scanners</SectionLabel>
                      <div className="flex flex-wrap gap-1">
                        {scanners.map((sc) => (
                          <Badge key={sc} label={sc} color="purple" />
                        ))}
                      </div>
                    </div>

                    {/* Target Namespace */}
                    <div>
                      <SectionLabel>
                        Target Namespace
                        <InfoIcon tip="Namespace where ImageScanJob CRs are created in the local cluster. All scan results land here regardless of which cluster the image was discovered from." />
                      </SectionLabel>
                      <span className="text-sm font-mono text-gray-700">{targetNS}</span>
                    </div>

                    {/* K8s Discovery */}
                    <div>
                      <SectionLabel>K8s Discovery</SectionLabel>
                      <span className={`text-sm font-medium ${spec.imageSources?.kubernetes?.enabled ? 'text-green-700' : 'text-gray-400'}`}>
                        {spec.imageSources?.kubernetes?.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </div>

                    {/* Notification */}
                    <div>
                      <SectionLabel>Notification Cooldown</SectionLabel>
                      <div className="flex items-center gap-1.5">
                        <span className="text-sm text-gray-700">{spec.notificationCooldown ?? '1h'}</span>
                        {spec.notificationPolicyRef && (
                          <Badge label={`policy: ${spec.notificationPolicyRef}`} color="yellow" />
                        )}
                      </div>
                    </div>
                  </div>

                  {/* ── Exclude Namespaces ────────────────────────────────── */}
                  {excludeNS.length > 0 && (
                    <div>
                      <SectionLabel>Excluded Namespaces</SectionLabel>
                      <div className="flex flex-wrap gap-1.5">
                        {excludeNS.map((ns) => (
                          <Badge key={ns} label={ns} color="red" />
                        ))}
                      </div>
                    </div>
                  )}

                  {/* ── Cluster Discovery ─────────────────────────────────── */}
                  <div>
                    <SectionLabel>
                      Cluster Targets
                      <InfoIcon tip="Remote clusters whose images are also scanned. Discovered images are merged with local images and scanned using the same pipeline." />
                    </SectionLabel>
                    {discoverAll ? (
                      <div className="flex items-center gap-2">
                        <Badge label="Auto-discover all connected clusters" color="blue" />
                      </div>
                    ) : clusterRefs.length > 0 ? (
                      <div className="flex flex-wrap gap-1.5">
                        {clusterRefs.map((ref) => (
                          <Badge key={ref} label={ref} color="blue" />
                        ))}
                      </div>
                    ) : (
                      <span className="text-sm text-gray-400">Local cluster only</span>
                    )}
                  </div>

                  {/* ── Status Bar ───────────────────────────────────────── */}
                  <div className="grid grid-cols-3 sm:grid-cols-6 gap-3 pt-4 border-t border-gray-100">
                    <StatCell label="Images"    value={s.totalImages ?? 0} />
                    <StatCell label="Active"    value={s.activeJobs ?? 0}    color="blue" />
                    <StatCell label="Completed" value={s.completedJobs ?? 0} color="green" />
                    <StatCell label="Failed"    value={s.failedJobs ?? 0}    color={failed > 0 ? 'red' : undefined} />
                    <div className="text-center">
                      <p className="text-xs text-gray-400 mb-1">Last Scan</p>
                      <p className="text-xs font-medium text-gray-700"><RelativeTime iso={s.lastScanTime} /></p>
                    </div>
                    <div className="text-center">
                      <p className="text-xs text-gray-400 mb-1">Last Notified</p>
                      <p className="text-xs font-medium text-gray-700"><RelativeTime iso={s.lastNotificationTime} /></p>
                    </div>
                  </div>

                  {/* ── Condition message ────────────────────────────────── */}
                  {readyCond?.message && (
                    <p className="text-xs text-gray-400 italic">{readyCond.message}</p>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function StatCell({ label, value, color }: { label: string; value: number; color?: 'blue' | 'green' | 'red' }) {
  const cls = color === 'blue' ? 'text-blue-600' : color === 'green' ? 'text-green-600' : color === 'red' ? 'text-red-600' : 'text-gray-800'
  return (
    <div className="text-center">
      <p className="text-xs text-gray-400 mb-1">{label}</p>
      <p className={`text-xl font-bold ${cls}`}>{value}</p>
    </div>
  )
}
