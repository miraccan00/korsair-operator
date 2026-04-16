import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import type { ImageInfo } from '../api/types'
import { LoadingSpinner } from '../components/LoadingSpinner'
import { ErrorMessage } from '../components/ErrorMessage'
import { useAutoRefresh } from '../hooks/useAutoRefresh'

// ── helpers ──────────────────────────────────────────────────────────────────

function phaseBadge(phase: string | undefined) {
  const cls: Record<string, string> = {
    Completed: 'bg-green-100 text-green-800',
    Running:   'bg-blue-100  text-blue-800',
    Failed:    'bg-red-100   text-red-800',
    Pending:   'bg-yellow-100 text-yellow-800',
    '':        'bg-gray-100  text-gray-500',
  }
  const label = phase || 'Pending'
  return (
    <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${cls[label] ?? cls['']}`}>
      {label}
    </span>
  )
}

function severityCell(value: number, color: string) {
  if (value === 0) return <span className="text-gray-300">—</span>
  return <span className={`font-semibold ${color}`}>{value}</span>
}

// ── component ─────────────────────────────────────────────────────────────────

export function ImagesPage() {
  const [images, setImages]         = useState<ImageInfo[]>([])
  const [loading, setLoading]       = useState(true)
  const [error, setError]           = useState<string | null>(null)
  const [selected, setSelected]     = useState<Set<string>>(new Set())
  const [scanning, setScanning]     = useState(false)
  const [scanResult, setScanResult] = useState<string | null>(null)

  // ── filters ──────────────────────────────────────────────────────────────
  const [phaseFilter,    setPhaseFilter]    = useState('All')
  const [registryFilter, setRegistryFilter] = useState('All')
  const [nsFilter,       setNsFilter]       = useState('All')
  const [clusterFilter,  setClusterFilter]  = useState('All')
  const [searchText,     setSearchText]     = useState('')

  // ── pagination ────────────────────────────────────────────────────────────
  const PAGE_SIZE = 50
  const [currentPage, setCurrentPage] = useState(1)

  const load = useCallback(async () => {
    try {
      const data = await api.getImages()
      setImages(data)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])
  useAutoRefresh(load, 20_000)

  // ── derived filter options ────────────────────────────────────────────────
  const registries = [...new Set(images.map(img => img.image.split('/')[0]))].sort()
  const namespaces = [...new Set(images.map(img => img.sourceNamespace ?? 'unknown'))].sort()
  const clusters   = [...new Set(images.map(img => img.clusterTarget || 'local'))].sort()

  const filtered = images.filter(img => {
    if (phaseFilter !== 'All' && (img.phase || '') !== phaseFilter) return false
    if (registryFilter !== 'All' && !img.image.startsWith(registryFilter)) return false
    if (nsFilter !== 'All' && (img.sourceNamespace ?? 'unknown') !== nsFilter) return false
    if (clusterFilter !== 'All' && (img.clusterTarget || 'local') !== clusterFilter) return false
    if (searchText && !img.image.toLowerCase().includes(searchText.toLowerCase())) return false
    return true
  })

  // Reset to page 1 whenever any filter changes.
  useEffect(() => { setCurrentPage(1) }, [phaseFilter, registryFilter, nsFilter, clusterFilter, searchText])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const paginated  = filtered.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE)

  // ── selection ─────────────────────────────────────────────────────────────
  const allFilteredKeys = filtered.map(img => img.image)
  const allSelected = allFilteredKeys.length > 0 && allFilteredKeys.every(k => selected.has(k))

  function toggleAll() {
    if (allSelected) {
      setSelected(prev => {
        const next = new Set(prev)
        allFilteredKeys.forEach(k => next.delete(k))
        return next
      })
    } else {
      setSelected(prev => new Set([...prev, ...allFilteredKeys]))
    }
  }

  function toggleOne(image: string) {
    setSelected(prev => {
      const next = new Set(prev)
      next.has(image) ? next.delete(image) : next.add(image)
      return next
    })
  }

  // ── manual scan ───────────────────────────────────────────────────────────
  async function triggerScan() {
    if (selected.size === 0) return
    setScanning(true)
    setScanResult(null)
    try {
      const res = await api.triggerScan({ images: [...selected] })
      setScanResult(`Created ${res.created.length} job(s), ${res.skipped.length} already running.`)
      setSelected(new Set())
      await load()
    } catch (e) {
      setScanResult(`Error: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setScanning(false)
    }
  }

  // ── delete a single ISJ ───────────────────────────────────────────────────
  async function deleteScan(ns: string, name: string) {
    try {
      await api.deleteScan(ns, name)
      await load()
    } catch (e) {
      setError(`Delete failed: ${e instanceof Error ? e.message : String(e)}`)
    }
  }

  if (loading) return <LoadingSpinner message="Loading images..." />
  if (error) return (
    <div className="p-6">
      <ErrorMessage message={error} onRetry={() => void load()} />
    </div>
  )

  return (
    <div className="p-6 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Images</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            {filtered.length} / {images.length} images
            {totalPages > 1 && ` · page ${currentPage}/${totalPages}`}
            {selected.size > 0 && ` · ${selected.size} selected`}
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => void load()}
            className="text-xs text-blue-600 hover:text-blue-800 border border-blue-300 hover:border-blue-500 px-3 py-1.5 rounded transition-colors"
          >
            ↻ Refresh
          </button>
          <button
            disabled={selected.size === 0 || scanning}
            onClick={() => void triggerScan()}
            className={`flex items-center gap-2 px-4 py-1.5 rounded text-sm font-medium transition-colors ${
              selected.size === 0 || scanning
                ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
                : 'bg-blue-600 hover:bg-blue-700 text-white'
            }`}
          >
            {scanning ? 'Scanning…' : `Scan ${selected.size > 0 ? `(${selected.size})` : 'Selected'}`}
          </button>
        </div>
      </div>

      {/* Scan result banner */}
      {scanResult && (
        <div className={`rounded-lg px-4 py-2 text-sm font-medium ${
          scanResult.startsWith('Error') ? 'bg-red-50 text-red-700' : 'bg-green-50 text-green-700'
        }`}>
          {scanResult}
          <button onClick={() => setScanResult(null)} className="ml-3 opacity-60 hover:opacity-100">✕</button>
        </div>
      )}

      {/* Filters */}
      <div className="flex flex-wrap gap-3 items-center">
        <input
          type="text"
          placeholder="Search image…"
          value={searchText}
          onChange={e => setSearchText(e.target.value)}
          className="text-sm border border-gray-300 rounded px-3 py-1 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500 w-60 font-mono"
        />

        {[
          { label: 'Phase',    value: phaseFilter,    set: setPhaseFilter,
            opts: ['Completed', 'Running', 'Failed', 'Pending'] },
          { label: 'Registry', value: registryFilter, set: setRegistryFilter, opts: registries },
          { label: 'Namespace',value: nsFilter,       set: setNsFilter,       opts: namespaces },
          { label: 'Cluster',  value: clusterFilter,  set: setClusterFilter,  opts: clusters },
        ].map(({ label, value, set, opts }) => (
          <div key={label} className="flex items-center gap-1.5">
            <label className="text-xs text-gray-500 font-medium">{label}:</label>
            <select
              value={value}
              onChange={e => set(e.target.value)}
              className="text-sm border border-gray-300 rounded px-2 py-1 bg-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <option value="All">All</option>
              {opts.map(o => <option key={o} value={o}>{o}</option>)}
            </select>
          </div>
        ))}

        {(phaseFilter !== 'All' || registryFilter !== 'All' || nsFilter !== 'All' || clusterFilter !== 'All' || searchText) && (
          <button
            onClick={() => { setPhaseFilter('All'); setRegistryFilter('All'); setNsFilter('All'); setClusterFilter('All'); setSearchText('') }}
            className="text-xs text-gray-500 hover:text-gray-800 border border-gray-300 rounded px-2 py-1"
          >
            Clear
          </button>
        )}
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-lg border border-gray-200 shadow-sm">
        <table className="min-w-full text-sm divide-y divide-gray-200">
          <thead className="bg-gray-50 text-xs text-gray-500 uppercase tracking-wider">
            <tr>
              <th className="px-3 py-3 w-8">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={toggleAll}
                  className="rounded border-gray-300"
                />
              </th>
              <th className="px-3 py-3 text-left">Image</th>
              <th className="px-3 py-3 text-left">Namespace</th>
              <th className="px-3 py-3 text-left">Cluster</th>
              <th className="px-3 py-3 text-left">Scanner</th>
              <th className="px-3 py-3 text-center">Status</th>
              <th className="px-3 py-3 text-center">C</th>
              <th className="px-3 py-3 text-center">H</th>
              <th className="px-3 py-3 text-center">M</th>
              <th className="px-3 py-3 text-center">L</th>
              <th className="px-3 py-3 text-left">Deployed</th>
              <th className="px-3 py-3 text-center w-16"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 bg-white">
            {paginated.length === 0 && (
              <tr>
                <td colSpan={12} className="text-center py-12 text-gray-400">
                  No images found
                </td>
              </tr>
            )}
            {paginated.map(img => {
              const isSelected = selected.has(img.image)
              return (
                <tr
                  key={`${img.isjNamespace}/${img.isjName}`}
                  className={`transition-colors ${isSelected ? 'bg-blue-50' : 'hover:bg-gray-50'}`}
                >
                  <td className="px-3 py-2">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleOne(img.image)}
                      className="rounded border-gray-300"
                    />
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-gray-800 max-w-xs">
                    <div className="truncate" title={img.image}>{img.image}</div>
                  </td>
                  <td className="px-3 py-2 text-xs text-gray-500 font-mono">
                    {img.sourceNamespace || '—'}
                  </td>
                  <td className="px-3 py-2 text-xs text-gray-500">
                    {img.clusterTarget || 'local'}
                  </td>
                  <td className="px-3 py-2 text-xs text-gray-500">
                    {img.scanner}
                  </td>
                  <td className="px-3 py-2 text-center">
                    {phaseBadge(img.phase)}
                  </td>
                  <td className="px-3 py-2 text-center">{severityCell(img.criticalCount, 'text-red-600')}</td>
                  <td className="px-3 py-2 text-center">{severityCell(img.highCount,     'text-orange-500')}</td>
                  <td className="px-3 py-2 text-center">{severityCell(img.mediumCount,   'text-yellow-500')}</td>
                  <td className="px-3 py-2 text-center">{severityCell(img.lowCount,      'text-blue-400')}</td>
                  <td className="px-3 py-2 text-xs text-gray-400 whitespace-nowrap">
                    {img.deployedAt ? new Date(img.deployedAt).toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' }) : '—'}
                  </td>
                  <td className="px-3 py-2 text-center">
                    {(img.phase === 'Failed' || img.phase === 'Completed') && (
                      <button
                        onClick={() => void deleteScan(img.isjNamespace, img.isjName)}
                        title="Delete scan job (allows re-scan)"
                        className="text-gray-400 hover:text-red-500 text-xs transition-colors"
                      >
                        ✕
                      </button>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {/* Pagination controls */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between px-1 py-2">
          <span className="text-sm text-gray-500">
            Showing {(currentPage - 1) * PAGE_SIZE + 1}–{Math.min(currentPage * PAGE_SIZE, filtered.length)} of {filtered.length}
          </span>
          <div className="flex items-center gap-1">
            <button
              disabled={currentPage === 1}
              onClick={() => setCurrentPage(1)}
              className="px-2 py-1 text-xs border border-gray-300 rounded hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
            >«</button>
            <button
              disabled={currentPage === 1}
              onClick={() => setCurrentPage(p => p - 1)}
              className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
            >← Prev</button>

            {Array.from({ length: Math.min(totalPages, 7) }, (_, i) => {
              // Show pages around current: always first, last, current ±2
              let page: number
              if (totalPages <= 7) {
                page = i + 1
              } else if (currentPage <= 4) {
                page = i + 1
              } else if (currentPage >= totalPages - 3) {
                page = totalPages - 6 + i
              } else {
                page = currentPage - 3 + i
              }
              return (
                <button
                  key={page}
                  onClick={() => setCurrentPage(page)}
                  className={`px-3 py-1 text-sm border rounded ${
                    page === currentPage
                      ? 'bg-blue-600 text-white border-blue-600'
                      : 'border-gray-300 hover:bg-gray-50'
                  }`}
                >{page}</button>
              )
            })}

            <button
              disabled={currentPage === totalPages}
              onClick={() => setCurrentPage(p => p + 1)}
              className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
            >Next →</button>
            <button
              disabled={currentPage === totalPages}
              onClick={() => setCurrentPage(totalPages)}
              className="px-2 py-1 text-xs border border-gray-300 rounded hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
            >»</button>
          </div>
        </div>
      )}
    </div>
  )
}
