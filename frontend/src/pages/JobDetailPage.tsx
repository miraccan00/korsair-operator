import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api } from '../api/client'
import type { ImageScanJob, ReportResponse } from '../api/types'
import { PhaseBadge } from '../components/PhaseBadge'
import { SeverityBadge } from '../components/SeverityBadge'
import { LoadingSpinner } from '../components/LoadingSpinner'
import { ErrorMessage } from '../components/ErrorMessage'

// ── Helpers ────────────────────────────────────────────────────────────────

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-2 py-2 border-b border-gray-100 last:border-0">
      <dt className="text-sm text-gray-500 font-medium">{label}</dt>
      <dd className="col-span-2 text-sm text-gray-800">{value}</dd>
    </div>
  )
}

function formatTime(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

const SEVERITY_ORDER: Record<string, number> = { CRITICAL: 4, HIGH: 3, MEDIUM: 2, LOW: 1 }

function severityClass(value: string): string {
  const v = value.toUpperCase()
  if (v === 'CRITICAL') return 'text-red-700 font-semibold'
  if (v === 'HIGH')     return 'text-orange-700 font-semibold'
  if (v === 'MEDIUM')   return 'text-yellow-700'
  if (v === 'LOW')      return 'text-gray-500'
  return ''
}

const CVE_RE = /CVE-\d{4}-\d+/i

// ── Severity bar chart ─────────────────────────────────────────────────────

function SeverityBar({ critical, high, medium, low }: { critical: number; high: number; medium: number; low: number }) {
  const total = critical + high + medium + low
  if (total === 0) return null
  const pct = (n: number) => `${Math.max(1, Math.round((n / total) * 100))}%`
  return (
    <div className="flex rounded-full overflow-hidden h-3 w-full bg-gray-100 gap-px" title={`Critical:${critical} High:${high} Medium:${medium} Low:${low}`}>
      {critical > 0 && <div className="bg-red-600"    style={{ width: pct(critical) }} />}
      {high     > 0 && <div className="bg-orange-500" style={{ width: pct(high) }} />}
      {medium   > 0 && <div className="bg-yellow-400" style={{ width: pct(medium) }} />}
      {low      > 0 && <div className="bg-gray-400"   style={{ width: pct(low) }} />}
    </div>
  )
}

// ── Sort helpers ───────────────────────────────────────────────────────────

function compareRows(a: string[], b: string[], col: number, dir: 'asc' | 'desc', isSeverityCol: boolean): number {
  const av = a[col] ?? ''
  const bv = b[col] ?? ''
  let cmp: number
  if (isSeverityCol) {
    cmp = (SEVERITY_ORDER[bv.toUpperCase()] ?? 0) - (SEVERITY_ORDER[av.toUpperCase()] ?? 0)
  } else {
    cmp = av.localeCompare(bv, undefined, { numeric: true })
  }
  return dir === 'asc' ? -cmp : cmp
}

// ── Main component ─────────────────────────────────────────────────────────

export function JobDetailPage() {
  const { namespace = 'korsair-system', name = '' } = useParams<{ namespace: string; name: string }>()
  const [job, setJob]         = useState<ImageScanJob | null>(null)
  const [report, setReport]   = useState<ReportResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState<string | null>(null)
  const [reportError, setReportError] = useState<string | null>(null)

  // UX state
  const [severityFilter, setSeverityFilter] = useState('All')
  const [search, setSearch]   = useState('')
  const [sortCol, setSortCol] = useState<number | null>(null)   // null = default (severity desc)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')
  const [copied, setCopied]   = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const jobs = await api.getJobs(namespace)
      const found = jobs.find((j) => j.metadata.name === name)
      setJob(found ?? null)
      if (!found) { setError(`Job "${name}" not found in namespace "${namespace}"`); return }
      setError(null)
      try {
        const r = await api.getJobReport(namespace, name)
        setReport(r)
        setReportError(null)
      } catch (e) {
        setReport(null)
        setReportError(e instanceof Error ? e.message : 'Report not available')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [namespace, name])

  useEffect(() => { void load() }, [load])

  const headers    = report?.rows[0] ?? []
  const allRows    = report?.rows.slice(1) ?? []
  const severityIdx = headers.findIndex((h) => h.toLowerCase() === 'severity')
  const vulnIdIdx   = headers.findIndex((h) => h.toLowerCase() === 'vulnerabilityid')
  const effectiveSortCol = sortCol ?? (severityIdx >= 0 ? severityIdx : 0)

  // Filter + sort in one memo
  const visibleRows = useMemo(() => {
    const q = search.toLowerCase()
    let rows = allRows.filter(row => {
      if (severityFilter !== 'All' && severityIdx >= 0) {
        if ((row[severityIdx] ?? '').toUpperCase() !== severityFilter) return false
      }
      if (q) return row.some(cell => cell.toLowerCase().includes(q))
      return true
    })
    rows = [...rows].sort((a, b) =>
      compareRows(a, b, effectiveSortCol, sortDir, effectiveSortCol === severityIdx)
    )
    return rows
  }, [allRows, severityFilter, search, effectiveSortCol, sortDir, severityIdx])

  const handleHeaderClick = (idx: number) => {
    if (sortCol === idx) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    } else {
      setSortCol(idx)
      setSortDir(idx === severityIdx ? 'desc' : 'asc')
    }
  }

  const copyImage = () => {
    if (!job) return
    navigator.clipboard.writeText(job.spec.image).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  if (loading) return <LoadingSpinner message="Loading job details..." />

  if (error || !job) {
    return (
      <div className="p-6 space-y-4">
        <Link to="/jobs" className="text-sm text-blue-600 hover:underline">← Back to jobs</Link>
        <ErrorMessage message={error ?? 'Job not found'} onRetry={() => void load()} />
      </div>
    )
  }

  const s = job.status
  const critical = s.criticalCount ?? 0
  const high     = s.highCount     ?? 0
  const medium   = s.mediumCount   ?? 0
  const low      = s.lowCount      ?? 0

  return (
    <div className="p-6 space-y-6">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-500">
        <Link to="/jobs" className="hover:text-blue-600 hover:underline">Scan Jobs</Link>
        <span>/</span>
        <span className="text-gray-800 font-medium font-mono">{name}</span>
      </div>

      {/* Job header */}
      <div className="flex items-start justify-between">
        <div className="space-y-1">
          <h1 className="text-xl font-bold text-gray-900 font-mono">{name}</h1>
          <div className="flex items-center gap-2">
            <p className="text-sm text-gray-500 font-mono break-all">{job.spec.image}</p>
            <button
              onClick={copyImage}
              title="Copy image reference"
              className="text-gray-400 hover:text-gray-600 shrink-0 text-xs border border-gray-200 rounded px-1.5 py-0.5"
            >
              {copied ? '✓ Copied' : 'Copy'}
            </button>
          </div>
        </div>
        <PhaseBadge phase={s.phase} />
      </div>

      {/* Severity summary + bar chart */}
      {s.phase === 'Completed' && (
        <div className="space-y-2">
          <div className="flex flex-wrap gap-2">
            <SeverityBadge severity="CRITICAL" count={critical} />
            <SeverityBadge severity="HIGH"     count={high} />
            <SeverityBadge severity="MEDIUM"   count={medium} />
            <SeverityBadge severity="LOW"      count={low} />
          </div>
          <SeverityBar critical={critical} high={high} medium={medium} low={low} />
        </div>
      )}

      {/* Details card */}
      <div className="rounded-lg border border-gray-200 bg-white shadow-sm p-5">
        <h2 className="text-base font-semibold text-gray-800 mb-3">Job Details</h2>
        <dl>
          <DetailRow label="Image"    value={<span className="font-mono text-xs break-all">{job.spec.image}</span>} />
          <DetailRow label="Scanner"  value={
            <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 border border-purple-200">
              {job.spec.scanner}
            </span>
          } />
          <DetailRow label="Config Ref"       value={job.spec.configRef} />
          <DetailRow label="Source Namespace" value={<span className="font-mono text-xs">{job.spec.sourceNamespace ?? namespace}</span>} />
          {job.metadata.labels?.['security.blacksyrius.com/cluster-target'] && (
            <DetailRow label="Cluster" value={
              <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-emerald-50 text-emerald-700 border border-emerald-200 font-mono">
                {job.metadata.labels['security.blacksyrius.com/cluster-target']}
              </span>
            } />
          )}
          <DetailRow label="Kubernetes Job" value={s.jobName ? <span className="font-mono text-xs">{s.jobName}</span> : '—'} />
          <DetailRow label="Started"        value={formatTime(s.startTime)} />
          <DetailRow label="Completed"      value={formatTime(s.completionTime)} />
          {s.message && <DetailRow label="Message" value={<span className="text-red-600">{s.message}</span>} />}
        </dl>
      </div>

      {/* CSV Report */}
      <div>
        <h2 className="text-base font-semibold text-gray-800 mb-3">Vulnerability Report</h2>

        {reportError && <p className="text-sm text-gray-400 italic">{reportError}</p>}

        {report && allRows.length === 0 && (
          <p className="text-sm text-green-600 font-medium">✅ No vulnerabilities found — image is clean.</p>
        )}

        {report && allRows.length > 0 && (
          <div className="space-y-3">
            {/* Filter + search bar */}
            <div className="flex flex-wrap items-center gap-2">
              {['All', 'CRITICAL', 'HIGH', 'MEDIUM', 'LOW'].map(f => (
                <button
                  key={f}
                  onClick={() => setSeverityFilter(f)}
                  className={`px-3 py-1 rounded-full text-xs font-medium border transition-colors ${
                    severityFilter === f
                      ? 'bg-gray-800 text-white border-gray-800'
                      : 'bg-white text-gray-600 border-gray-300 hover:border-gray-500'
                  }`}
                >
                  {f}
                </button>
              ))}
              <input
                type="text"
                placeholder="Search CVE ID, library, title…"
                value={search}
                onChange={e => setSearch(e.target.value)}
                className="ml-auto border border-gray-300 rounded-lg px-3 py-1 text-xs focus:outline-none focus:ring-2 focus:ring-blue-500 w-56"
              />
            </div>

            {/* Table */}
            <div className="overflow-x-auto rounded-lg border border-gray-200 shadow-sm">
              <table className="min-w-full divide-y divide-gray-200 text-xs">
                <thead className="bg-gray-50">
                  <tr>
                    {headers.map((h, idx) => {
                      const active = effectiveSortCol === idx
                      return (
                        <th
                          key={h}
                          onClick={() => handleHeaderClick(idx)}
                          className="px-3 py-2 text-left font-medium text-gray-600 whitespace-nowrap cursor-pointer select-none hover:bg-gray-100"
                        >
                          {h}
                          {active && <span className="ml-1 text-gray-400">{sortDir === 'asc' ? '↑' : '↓'}</span>}
                        </th>
                      )
                    })}
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 bg-white">
                  {visibleRows.length === 0 ? (
                    <tr>
                      <td colSpan={headers.length} className="px-3 py-6 text-center text-gray-400">
                        No results match your filter.
                      </td>
                    </tr>
                  ) : (
                    visibleRows.map((row, i) => (
                      <tr key={i} className="hover:bg-gray-50">
                        {row.map((cell, j) => {
                          // Severity column
                          if (j === severityIdx) {
                            return (
                              <td key={j} className={`px-3 py-2 whitespace-nowrap ${severityClass(cell)}`}>
                                {cell || '—'}
                              </td>
                            )
                          }
                          // VulnerabilityID column — link CVEs to NVD
                          if (j === vulnIdIdx && CVE_RE.test(cell)) {
                            return (
                              <td key={j} className="px-3 py-2 whitespace-nowrap">
                                <a
                                  href={`https://nvd.nist.gov/vuln/detail/${cell}`}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="text-blue-600 hover:underline"
                                >
                                  {cell}
                                </a>
                              </td>
                            )
                          }
                          return (
                            <td key={j} className="px-3 py-2 whitespace-nowrap text-gray-700">
                              {cell || '—'}
                            </td>
                          )
                        })}
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
              <div className="px-4 py-2 bg-gray-50 border-t border-gray-200 text-xs text-gray-400">
                {visibleRows.length} of {allRows.length} vulnerabilities · ConfigMap: {report.configMapName}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
