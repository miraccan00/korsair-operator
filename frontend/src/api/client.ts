import type {
  SecurityScanConfig,
  ImageScanJob,
  ImageInfo,
  SummaryResponse,
  ReportResponse,
  ClusterTarget,
  ScanRequest,
  ScanResponse,
} from './types'

const BASE = '/api/v1'

async function apiFetch<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText })) as { error?: string }
    throw new Error(err.error ?? `HTTP ${res.status}`)
  }
  return res.json() as Promise<T>
}

export const api = {
  getSummary: () =>
    apiFetch<SummaryResponse>('/summary'),

  getConfigs: () =>
    apiFetch<SecurityScanConfig[]>('/configs'),

  getJobs: (namespace?: string) => {
    const query = namespace ? `?namespace=${encodeURIComponent(namespace)}` : ''
    return apiFetch<ImageScanJob[]>(`/jobs${query}`)
  },

  getJobReport: (namespace: string, name: string) =>
    apiFetch<ReportResponse>(`/jobs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/report`),

  getClusters: () =>
    apiFetch<ClusterTarget[]>('/clusters'),

  createCluster: (displayName: string, kubeconfigFile: File): Promise<{ name: string; status: string }> => {
    const form = new FormData()
    form.append('displayName', displayName)
    form.append('kubeconfig', kubeconfigFile)
    return fetch(`${BASE}/clusters`, { method: 'POST', body: form })
      .then(async res => {
        if (!res.ok) {
          const err = await res.json().catch(() => ({ error: res.statusText })) as { error?: string }
          throw new Error(err.error ?? `HTTP ${res.status}`)
        }
        return res.json()
      })
  },

  deleteCluster: async (name: string): Promise<void> => {
    const res = await fetch(`${BASE}/clusters/${encodeURIComponent(name)}`, { method: 'DELETE' })
    if (!res.ok && res.status !== 204) {
      const err = await res.json().catch(() => ({ error: res.statusText })) as { error?: string }
      throw new Error(err.error ?? `HTTP ${res.status}`)
    }
  },

  getImages: () =>
    apiFetch<ImageInfo[]>('/images'),

  triggerScan: (req: ScanRequest): Promise<ScanResponse> =>
    fetch(`${BASE}/scan`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }).then(async res => {
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText })) as { error?: string }
        throw new Error(err.error ?? `HTTP ${res.status}`)
      }
      return res.json() as Promise<ScanResponse>
    }),

  deleteScan: async (namespace: string, name: string): Promise<void> => {
    const res = await fetch(
      `${BASE}/scan/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`,
      { method: 'DELETE' }
    )
    if (!res.ok && res.status !== 204) {
      const err = await res.json().catch(() => ({ error: res.statusText })) as { error?: string }
      throw new Error(err.error ?? `HTTP ${res.status}`)
    }
  },
}
