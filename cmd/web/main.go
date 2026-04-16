/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main is the entry point for the BSO web dashboard binary.
// It exposes REST API endpoints that read BSO custom resources from Kubernetes.
// When built with -tags webui it also serves the embedded React SPA.
// When built without the tag (local dev) only the /api/v1/* routes are active;
// the Vite dev server (npm run dev) serves the frontend and proxies /api to this process.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
	"github.com/miraccan00/korsair-operator/cmd/web/static"
)

// operatorNamespace is where ClusterTarget CRs and their kubeconfig Secrets are managed.
const operatorNamespace = "korsair-system"

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(securityv1alpha1.AddToScheme(scheme))
}

// summaryResponse mirrors the JSON shape returned by GET /api/v1/summary.
type summaryResponse struct {
	TotalImages   int        `json:"totalImages"`
	TotalJobs     int        `json:"totalJobs"`
	ActiveJobs    int        `json:"activeJobs"`
	CompletedJobs int        `json:"completedJobs"`
	FailedJobs    int        `json:"failedJobs"`
	TotalCritical int        `json:"totalCritical"`
	TotalHigh     int        `json:"totalHigh"`
	TotalMedium   int        `json:"totalMedium"`
	TotalLow      int        `json:"totalLow"`
	LastScanTime  *time.Time `json:"lastScanTime,omitempty"`
	ConfigCount   int        `json:"configCount"`
}

// reportResponse mirrors the JSON shape returned by GET /api/v1/jobs/{ns}/{name}/report.
type reportResponse struct {
	ConfigMapName string     `json:"configMapName"`
	RawCSV        string     `json:"rawCSV"`
	Rows          [][]string `json:"rows"`
}

func main() {
	var listenAddr string
	var kubeconfig string
	flag.StringVar(&listenAddr, "listen-addr", ":8090", "HTTP server listen address")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (empty = in-cluster config)")
	flag.Parse()

	k8sClient := buildK8sClient(kubeconfig)

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 60*time.Second)
	db, err := openDB(dbCtx)
	dbCancel()
	if err != nil {
		slog.Error("DB init failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	slog.Info("DB connected and schema ready")

	internalToken := os.Getenv("KORSAIR_INTERNAL_TOKEN")

	mux := http.NewServeMux()

	// API routes — registered before the SPA catch-all
	mux.HandleFunc("GET /api/v1/summary", handleSummary(k8sClient))
	mux.HandleFunc("GET /api/v1/configs", handleConfigs(k8sClient))
	mux.HandleFunc("GET /api/v1/jobs", handleJobs(k8sClient))
	mux.HandleFunc("GET /api/v1/jobs/{namespace}/{name}/report", handleJobReport(db))
	mux.HandleFunc("PUT /api/v1/jobs/{namespace}/{name}/report", handlePutJobReport(db, internalToken))
	mux.HandleFunc("GET /api/v1/images", handleImages(k8sClient))
	mux.HandleFunc("POST /api/v1/scan", handleTriggerScan(k8sClient))
	mux.HandleFunc("DELETE /api/v1/scan/{namespace}/{name}", handleDeleteScan(k8sClient))
	mux.HandleFunc("POST /api/v1/configs/{name}/reset", handleResetConfig(k8sClient))
	mux.HandleFunc("GET /api/v1/clusters", handleGetClusters(k8sClient))
	mux.HandleFunc("POST /api/v1/clusters", handleCreateCluster(k8sClient))
	mux.HandleFunc("DELETE /api/v1/clusters/{name}", handleDeleteCluster(k8sClient))

	// SPA static file serving — only when compiled with -tags webui (embedded React build).
	// In dev mode (no tag) only the /api/v1/* routes above are active.
	if static.HasUI {
		distFS, err := fs.Sub(static.Files, "dist")
		if err != nil {
			slog.Error("Failed to sub embedded FS", "err", err)
			os.Exit(1)
		}
		fileServer := http.FileServer(http.FS(distFS))
		mux.HandleFunc("/", spaHandler(fileServer))
		slog.Info("Serving embedded React SPA")
	} else {
		slog.Info("API-only mode (built without -tags webui); use 'npm run dev' for the frontend")
	}

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("BSO Web dashboard starting", "addr", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down web server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown error", "err", err)
	}
}

// buildK8sClient creates a controller-runtime client.
// Resolution order (matches kubectl behaviour):
//  1. --kubeconfig flag (explicit path)
//  2. KUBECONFIG environment variable
//  3. ~/.kube/config (default kubeconfig)
//  4. In-cluster service account token (production pod)
func buildK8sClient(kubeconfigPath string) client.Client {
	var cfg *rest.Config
	var err error

	if kubeconfigPath != "" {
		// Explicit --kubeconfig flag takes highest priority.
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		// Use the same loading rules as kubectl: KUBECONFIG env var → ~/.kube/config.
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{},
		)
		cfg, err = kubeConfig.ClientConfig()
		if err != nil {
			// No local kubeconfig found → try in-cluster service account (production).
			slog.Info("No kubeconfig found, falling back to in-cluster config")
			cfg, err = rest.InClusterConfig()
		}
	}
	if err != nil {
		slog.Error("Failed to build Kubernetes config", "err", err)
		os.Exit(1)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		slog.Error("Failed to create Kubernetes client", "err", err)
		os.Exit(1)
	}
	return c
}

// ── HTTP Handlers ─────────────────────────────────────────────────────────

// handleSummary serves GET /api/v1/summary — aggregated statistics across all ISJs and SSCs.
func handleSummary(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var jobs securityv1alpha1.ImageScanJobList
		if err := c.List(r.Context(), &jobs); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing ImageScanJobs: %v", err))
			return
		}

		var configs securityv1alpha1.SecurityScanConfigList
		if err := c.List(r.Context(), &configs); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing SecurityScanConfigs: %v", err))
			return
		}

		resp := computeSummary(jobs.Items, configs.Items)
		writeJSON(w, resp)
	}
}

// handleConfigs serves GET /api/v1/configs — list all SecurityScanConfig resources.
func handleConfigs(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var list securityv1alpha1.SecurityScanConfigList
		if err := c.List(r.Context(), &list); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing SecurityScanConfigs: %v", err))
			return
		}
		writeJSON(w, list.Items)
	}
}

// handleJobs serves GET /api/v1/jobs[?namespace=korsair-system] — list ImageScanJob resources.
func handleJobs(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := r.URL.Query().Get("namespace")
		opts := []client.ListOption{}
		if ns != "" {
			opts = append(opts, client.InNamespace(ns))
		}

		var list securityv1alpha1.ImageScanJobList
		if err := c.List(r.Context(), &list, opts...); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing ImageScanJobs: %v", err))
			return
		}
		writeJSON(w, list.Items)
	}
}

// handleJobReport serves GET /api/v1/jobs/{namespace}/{name}/report —
// reads CVE rows from Postgres and returns them as JSON (with a server-side
// generated CSV string for backwards compatibility with the UI).
func handleJobReport(db *sql.DB) http.HandlerFunc {
	header := []string{
		"Image", "Target", "Library", "VulnerabilityID",
		"Severity", "Status", "InstalledVersion", "FixedVersion", "Title",
	}
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")
		name := r.PathValue("name")

		const q = `SELECT image, target, library, vulnerability_id, severity, status,
			installed_version, fixed_version, title
			FROM scan_report_rows
			WHERE isj_namespace = $1 AND isj_name = $2
			ORDER BY severity, vulnerability_id`

		rs, err := db.QueryContext(r.Context(), q, namespace, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("query: %v", err))
			return
		}
		defer func() { _ = rs.Close() }()

		rows := [][]string{header}
		var buf strings.Builder
		cw := csv.NewWriter(&buf)
		_ = cw.Write(header)

		for rs.Next() {
			var rec reportRow
			if err := rs.Scan(&rec.Image, &rec.Target, &rec.Library, &rec.VulnerabilityID,
				&rec.Severity, &rec.Status, &rec.InstalledVersion, &rec.FixedVersion, &rec.Title); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan: %v", err))
				return
			}
			row := []string{rec.Image, rec.Target, rec.Library, rec.VulnerabilityID, rec.Severity,
				rec.Status, rec.InstalledVersion, rec.FixedVersion, rec.Title}
			rows = append(rows, row)
			_ = cw.Write(row)
		}
		if err := rs.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate: %v", err))
			return
		}
		cw.Flush()

		if len(rows) == 1 {
			writeError(w, http.StatusNotFound, fmt.Sprintf("no report rows for %s/%s", namespace, name))
			return
		}

		writeJSON(w, reportResponse{
			ConfigMapName: name + "-report",
			RawCSV:        buf.String(),
			Rows:          rows,
		})
	}
}

// handlePutJobReport accepts a batch of CVE rows from the operator and
// replaces any existing rows for this ImageScanJob. Authenticated via a
// shared-secret header when KORSAIR_INTERNAL_TOKEN is set.
func handlePutJobReport(db *sql.DB, internalToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if internalToken != "" && r.Header.Get("X-Korsair-Token") != internalToken {
			writeError(w, http.StatusUnauthorized, "invalid internal token")
			return
		}

		namespace := r.PathValue("namespace")
		name := r.PathValue("name")

		var body reportWriteRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("decode body: %v", err))
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("begin: %v", err))
			return
		}
		defer func() { _ = tx.Rollback() }()

		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO scan_reports (isj_namespace, isj_name, scanner, updated_at)
			 VALUES ($1, $2, $3, now())
			 ON CONFLICT (isj_namespace, isj_name)
			 DO UPDATE SET scanner = EXCLUDED.scanner, updated_at = now()`,
			namespace, name, body.Scanner); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("upsert report: %v", err))
			return
		}

		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM scan_report_rows WHERE isj_namespace = $1 AND isj_name = $2`,
			namespace, name); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete rows: %v", err))
			return
		}

		stmt, err := tx.PrepareContext(r.Context(),
			`INSERT INTO scan_report_rows (isj_namespace, isj_name, image, target, library,
				vulnerability_id, severity, status, installed_version, fixed_version, title)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("prepare: %v", err))
			return
		}
		defer func() { _ = stmt.Close() }()

		for _, rec := range body.Rows {
			if _, err := stmt.ExecContext(r.Context(),
				namespace, name, rec.Image, rec.Target, rec.Library, rec.VulnerabilityID,
				rec.Severity, rec.Status, rec.InstalledVersion, rec.FixedVersion, rec.Title); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("insert row: %v", err))
				return
			}
		}

		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("commit: %v", err))
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ── Image list & manual scan handlers ─────────────────────────────────────

// imageInfo is the JSON shape returned per discovered image in GET /api/v1/images.
type imageInfo struct {
	Image           string     `json:"image"`
	Scanner         string     `json:"scanner"`
	SourceNamespace string     `json:"sourceNamespace,omitempty"`
	ClusterTarget   string     `json:"clusterTarget,omitempty"`
	Phase           string     `json:"phase,omitempty"`
	CriticalCount   int        `json:"criticalCount"`
	HighCount       int        `json:"highCount"`
	MediumCount     int        `json:"mediumCount"`
	LowCount        int        `json:"lowCount"`
	ISJName         string     `json:"isjName"`
	ISJNamespace    string     `json:"isjNamespace"`
	StartTime       *time.Time `json:"startTime,omitempty"`
	CompletionTime  *time.Time `json:"completionTime,omitempty"`
	DeployedAt      *time.Time `json:"deployedAt,omitempty"`
}

// handleImages serves GET /api/v1/images — all discovered images with their current scan status.
func handleImages(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var list securityv1alpha1.ImageScanJobList
		if err := c.List(r.Context(), &list); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing ImageScanJobs: %v", err))
			return
		}
		result := make([]imageInfo, 0, len(list.Items))
		for i := range list.Items {
			isj := &list.Items[i]
			info := imageInfo{
				Image:           isj.Spec.Image,
				Scanner:         isj.Spec.Scanner,
				SourceNamespace: isj.Spec.SourceNamespace,
				ClusterTarget:   isj.Labels["security.blacksyrius.com/cluster-target"],
				Phase:           string(isj.Status.Phase),
				CriticalCount:   isj.Status.CriticalCount,
				HighCount:       isj.Status.HighCount,
				MediumCount:     isj.Status.MediumCount,
				LowCount:        isj.Status.LowCount,
				ISJName:         isj.Name,
				ISJNamespace:    isj.Namespace,
			}
			if isj.Status.StartTime != nil {
				t := isj.Status.StartTime.Time
				info.StartTime = &t
			}
			if isj.Status.CompletionTime != nil {
				t := isj.Status.CompletionTime.Time
				info.CompletionTime = &t
			}
			if raw := isj.Annotations["security.blacksyrius.com/deployed-at"]; raw != "" {
				if t, err := time.Parse(time.RFC3339, raw); err == nil {
					info.DeployedAt = &t
				}
			}
			result = append(result, info)
		}
		// Sort by deployedAt ascending (oldest deployment first); items without a
		// deployedAt timestamp are placed at the end.
		sort.Slice(result, func(i, j int) bool {
			ti := result[i].DeployedAt
			tj := result[j].DeployedAt
			if ti == nil && tj == nil {
				return result[i].Image < result[j].Image
			}
			if ti == nil {
				return false
			}
			if tj == nil {
				return true
			}
			return ti.Before(*tj)
		})
		writeJSON(w, result)
	}
}

// scanRequest is the JSON body accepted by POST /api/v1/scan.
type scanRequest struct {
	Images          []string `json:"images"`
	Scanner         string   `json:"scanner"`
	TargetNamespace string   `json:"targetNamespace"`
}

// handleTriggerScan serves POST /api/v1/scan.
// Creates ImageScanJob CRs for each image in the request body.
func handleTriggerScan(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req scanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
			return
		}
		if len(req.Images) == 0 {
			writeError(w, http.StatusBadRequest, "images list is required and must not be empty")
			return
		}
		scanner := req.Scanner
		if scanner == "" {
			scanner = "trivy"
		}
		ns := req.TargetNamespace
		if ns == "" {
			ns = operatorNamespace
		}

		created := []string{}
		skipped := []string{}
		for _, image := range req.Images {
			jobName := manualScanJobName(image, scanner)
			existing := &securityv1alpha1.ImageScanJob{}
			if err := c.Get(r.Context(), types.NamespacedName{Name: jobName, Namespace: ns}, existing); err == nil {
				skipped = append(skipped, jobName) // already exists
				continue
			}
			isj := &securityv1alpha1.ImageScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: ns,
					Labels: map[string]string{
						"security.blacksyrius.com/scanner": scanner,
						"security.blacksyrius.com/config":  "manual",
					},
					Annotations: map[string]string{
						"security.blacksyrius.com/image": image,
					},
				},
				Spec: securityv1alpha1.ImageScanJobSpec{
					Image:     image,
					Scanner:   scanner,
					Source:    "manual",
					ConfigRef: "manual",
				},
			}
			if err := c.Create(r.Context(), isj); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("creating ISJ for %q: %v", image, err))
				return
			}
			created = append(created, jobName)
		}

		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{
			"created": created,
			"skipped": skipped,
		})
	}
}

// handleDeleteScan serves DELETE /api/v1/scan/{namespace}/{name}.
// Deletes the named ImageScanJob (and its owned batch Job via owner reference).
func handleDeleteScan(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := r.PathValue("namespace")
		name := r.PathValue("name")
		isj := &securityv1alpha1.ImageScanJob{}
		if err := c.Get(r.Context(), types.NamespacedName{Namespace: ns, Name: name}, isj); err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("ImageScanJob %q not found in namespace %q", name, ns))
			return
		}
		if err := c.Delete(r.Context(), isj); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("deleting ISJ: %v", err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// manualScanJobName produces a deterministic ISJ name for a manually triggered scan.
// Uses SHA-256 of the image string (no digest available at trigger time).
func manualScanJobName(image, scanner string) string {
	h := sha256.Sum256([]byte(image))
	return fmt.Sprintf("scan-%x-%s", h[:8], scanner)
}

// handleResetConfig serves POST /api/v1/configs/{name}/reset.
// It sets the reset annotation on the SecurityScanConfig, which causes the
// controller to delete all owned ImageScanJobs and start fresh.
func handleResetConfig(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var cfgList securityv1alpha1.SecurityScanConfigList
		if err := c.List(r.Context(), &cfgList); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing SecurityScanConfigs: %v", err))
			return
		}

		var target *securityv1alpha1.SecurityScanConfig
		for i := range cfgList.Items {
			if cfgList.Items[i].Name == name {
				target = &cfgList.Items[i]
				break
			}
		}
		if target == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("SecurityScanConfig %q not found", name))
			return
		}

		patch := client.MergeFrom(target.DeepCopy())
		if target.Annotations == nil {
			target.Annotations = map[string]string{}
		}
		target.Annotations["security.blacksyrius.com/reset"] = "true"
		if err := c.Patch(r.Context(), target, patch); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("patching reset annotation: %v", err))
			return
		}

		slog.Info("Reset annotation applied", "config", name)
		w.WriteHeader(http.StatusAccepted)
	}
}

// ── Cluster management handlers ───────────────────────────────────────────

// clusterResponse is the JSON shape returned for each ClusterTarget.
type clusterResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	Phase       string    `json:"phase"`
	NodeCount   int       `json:"nodeCount"`
	Message     string    `json:"message,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// handleGetClusters serves GET /api/v1/clusters — lists ClusterTargets in korsair-system.
func handleGetClusters(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var list securityv1alpha1.ClusterTargetList
		if err := c.List(r.Context(), &list, client.InNamespace(operatorNamespace)); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing ClusterTargets: %v", err))
			return
		}
		resp := make([]clusterResponse, 0, len(list.Items))
		for _, ct := range list.Items {
			resp = append(resp, clusterResponse{
				Name:        ct.Name,
				DisplayName: ct.Spec.DisplayName,
				Phase:       string(ct.Status.Phase),
				NodeCount:   ct.Status.NodeCount,
				Message:     ct.Status.Message,
				CreatedAt:   ct.CreationTimestamp.Time,
			})
		}
		writeJSON(w, resp)
	}
}

// handleCreateCluster serves POST /api/v1/clusters.
// Accepts multipart/form-data with fields: "displayName" (text) + "kubeconfig" (file).
// Creates a kubeconfig Secret and a ClusterTarget CR in korsair-system.
func handleCreateCluster(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil { // 1 MiB limit
			writeError(w, http.StatusBadRequest, fmt.Sprintf("parsing form: %v", err))
			return
		}

		displayName := strings.TrimSpace(r.FormValue("displayName"))
		if displayName == "" {
			writeError(w, http.StatusBadRequest, "displayName is required")
			return
		}

		file, _, err := r.FormFile("kubeconfig")
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("reading kubeconfig file: %v", err))
			return
		}
		defer func() { _ = file.Close() }()

		kubeconfigBytes, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("reading kubeconfig: %v", err))
			return
		}

		// Derive a DNS-safe name from displayName.
		clusterName := strings.ToLower(strings.ReplaceAll(displayName, " ", "-"))
		secretName := "ct-kubeconfig-" + clusterName

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: operatorNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by":     "korsair-operator",
					"security.blacksyrius.com/purpose": "cluster-target-kubeconfig",
				},
			},
			Data: map[string][]byte{"kubeconfig": kubeconfigBytes},
		}
		if err := c.Create(r.Context(), secret); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("creating kubeconfig secret: %v", err))
			return
		}

		ct := &securityv1alpha1.ClusterTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: operatorNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "korsair-operator",
				},
			},
			Spec: securityv1alpha1.ClusterTargetSpec{
				DisplayName:         displayName,
				KubeconfigSecretRef: corev1.LocalObjectReference{Name: secretName},
			},
		}
		if err := c.Create(r.Context(), ct); err != nil {
			_ = c.Delete(r.Context(), secret) // rollback secret
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("creating ClusterTarget: %v", err))
			return
		}

		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"name": clusterName, "status": "created"})
	}
}

// handleDeleteCluster serves DELETE /api/v1/clusters/{name}.
// Deletes the ClusterTarget and its associated kubeconfig Secret.
func handleDeleteCluster(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		ct := &securityv1alpha1.ClusterTarget{}
		if err := c.Get(r.Context(), types.NamespacedName{Namespace: operatorNamespace, Name: name}, ct); err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("ClusterTarget %q not found", name))
			return
		}

		// Delete the associated Secret first (best-effort).
		secret := &corev1.Secret{}
		if err := c.Get(r.Context(), types.NamespacedName{
			Namespace: operatorNamespace,
			Name:      ct.Spec.KubeconfigSecretRef.Name,
		}, secret); err == nil {
			_ = c.Delete(r.Context(), secret)
		}

		if err := c.Delete(r.Context(), ct); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("deleting ClusterTarget: %v", err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// spaHandler serves the embedded React SPA. For non-asset paths (no dot extension),
// it rewrites the request to "/" so React Router can handle client-side navigation.
func spaHandler(fileServer http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// If the path looks like a React route (no file extension), serve index.html
		if path != "/" && !strings.Contains(path, ".") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}
}

// ── Aggregation ───────────────────────────────────────────────────────────

func computeSummary(
	jobs []securityv1alpha1.ImageScanJob,
	configs []securityv1alpha1.SecurityScanConfig,
) summaryResponse {
	s := summaryResponse{
		TotalJobs:   len(jobs),
		ConfigCount: len(configs),
	}

	for i := range jobs {
		j := &jobs[i]
		switch j.Status.Phase {
		case securityv1alpha1.ImageScanJobPhaseRunning:
			s.ActiveJobs++
		case securityv1alpha1.ImageScanJobPhaseCompleted:
			s.CompletedJobs++
			s.TotalCritical += j.Status.CriticalCount
			s.TotalHigh += j.Status.HighCount
			s.TotalMedium += j.Status.MediumCount
			s.TotalLow += j.Status.LowCount
		case securityv1alpha1.ImageScanJobPhaseFailed:
			s.FailedJobs++
		}
	}

	uniqueImages := make(map[string]struct{})
	for i := range jobs {
		if jobs[i].Spec.Image != "" {
			uniqueImages[jobs[i].Spec.Image] = struct{}{}
		}
	}
	s.TotalImages = len(uniqueImages)

	for i := range configs {
		cfg := &configs[i]
		if cfg.Status.LastScanTime != nil {
			t := cfg.Status.LastScanTime.Time
			if s.LastScanTime == nil || t.After(*s.LastScanTime) {
				s.LastScanTime = &t
			}
		}
	}

	return s
}

// ── HTTP helpers ──────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("Failed to marshal JSON response", "err", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if _, err := w.Write(data); err != nil {
		slog.Error("Failed to write JSON response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
