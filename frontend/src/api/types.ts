// Korsair API type definitions — mirrors Go structs in api/v1alpha1/

export type ImageScanJobPhase = 'Pending' | 'Running' | 'Completed' | 'Failed'

export interface ObjectMeta {
  name: string
  namespace?: string
  creationTimestamp: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface Condition {
  type: string
  status: 'True' | 'False' | 'Unknown'
  reason?: string
  message?: string
  lastTransitionTime?: string
  observedGeneration?: number
}

// ── SecurityScanConfig ─────────────────────────────────────────────────────

export interface SecurityScanConfigSpec {
  imageSources: {
    kubernetes: {
      enabled: boolean
      excludeNamespaces?: string[]
    }
    clusterTargetRefs?: string[]
    discoverAllClusters?: boolean
  }
  schedule?: string
  scanners?: string[]
  targetNamespace?: string
  notificationCooldown?: string
  notificationPolicyRef?: string
}

export interface SecurityScanConfigStatus {
  lastScanTime?: string
  totalImages?: number
  activeJobs?: number
  completedJobs?: number
  failedJobs?: number
  lastNotificationTime?: string
  notifiedImageCount?: number
  conditions?: Condition[]
}

export interface SecurityScanConfig {
  apiVersion: string
  kind: string
  metadata: ObjectMeta
  spec: SecurityScanConfigSpec
  status: SecurityScanConfigStatus
}

// ── ImageScanJob ───────────────────────────────────────────────────────────

export interface ImageScanJobSpec {
  image: string
  imageDigest?: string
  scanner: 'trivy' | 'grype'
  source: string
  configRef: string
  sourceNamespace?: string
}

export interface ImageScanJobStatus {
  phase?: ImageScanJobPhase
  jobName?: string
  startTime?: string
  completionTime?: string
  criticalCount?: number
  highCount?: number
  mediumCount?: number
  lowCount?: number
  message?: string
  conditions?: Condition[]
}

export interface ImageScanJob {
  apiVersion: string
  kind: string
  metadata: ObjectMeta
  spec: ImageScanJobSpec
  status: ImageScanJobStatus
}

// ── API Response Types ─────────────────────────────────────────────────────

export interface SummaryResponse {
  totalImages: number
  totalJobs: number
  activeJobs: number
  completedJobs: number
  failedJobs: number
  totalCritical: number
  totalHigh: number
  totalMedium: number
  totalLow: number
  lastScanTime?: string
  configCount: number
}

export interface ReportResponse {
  configMapName: string
  rawCSV: string
  rows: string[][] // [header_row, ...data_rows]
}

// ── Image list & manual scan ───────────────────────────────────────────────

export interface ImageInfo {
  image: string
  scanner: string
  sourceNamespace?: string
  clusterTarget?: string
  phase?: ImageScanJobPhase | ''
  criticalCount: number
  highCount: number
  mediumCount: number
  lowCount: number
  isjName: string
  isjNamespace: string
  startTime?: string
  completionTime?: string
  deployedAt?: string
}

export interface ScanRequest {
  images: string[]
  scanner?: string
  targetNamespace?: string
}

export interface ScanResponse {
  created: string[]
  skipped: string[]
}

// ── ClusterTarget ──────────────────────────────────────────────────────────

export type ClusterTargetPhase = 'Connected' | 'Error' | ''

export interface ClusterTarget {
  name: string
  displayName: string
  phase: ClusterTargetPhase
  nodeCount: number
  message?: string
  createdAt: string
}
