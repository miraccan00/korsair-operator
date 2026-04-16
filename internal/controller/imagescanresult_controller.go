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

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
)

// mustParseQuantity parses a resource quantity string and panics on error.
// Used only for static values defined at compile time (e.g. "512Mi").
func mustParseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

const (
	trivyJobSuffix = "-trivy"
	grypeJobSuffix = "-grype"
	scannerGrype   = "grype"
)

// scannerJobSuffix returns the Job name suffix for the given scanner.
func scannerJobSuffix(scanner string) string {
	if scanner == scannerGrype {
		return grypeJobSuffix
	}
	return trivyJobSuffix
}

// rewriteRegistry replaces the registry host of an image reference when it
// matches source.  The registry is the portion before the first '/' separator.
//
// Examples:
//
//	rewriteRegistry("BSO_REGISTRY_REWRITE_SOURCE.company_name.com/app/api:v1", "BSO_REGISTRY_REWRITE_SOURCE.company_name.com", "registry.company_name.com")
//	  → "registry.company_name.com/app/api:v1"
//
//	rewriteRegistry("nginx:latest", ...) → "nginx:latest"  (no registry prefix)
func rewriteRegistry(image, source, target string) string {
	if source == "" || target == "" || source == target {
		return image
	}
	registry, rest, ok := strings.Cut(image, "/")
	if !ok {
		return image // no registry prefix (e.g. "nginx:latest")
	}
	if registry == source {
		return target + "/" + rest
	}
	return image
}

// trivyOutput represents the top-level structure of Trivy's JSON output.
type trivyOutput struct {
	Results []trivyResult `json:"Results"`
}

// trivyResult holds the vulnerabilities found in a single scan target (OS, language packages, etc.).
type trivyResult struct {
	Target          string      `json:"Target"`
	Vulnerabilities []trivyVuln `json:"Vulnerabilities"`
}

// trivyVuln represents a single vulnerability entry with all fields needed for CSV export.
type trivyVuln struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Severity         string `json:"Severity"`
	Status           string `json:"Status"`
	Title            string `json:"Title"`
}

// grypeOutput represents the top-level structure of Grype's JSON output.
type grypeOutput struct {
	Matches []grypeMatch `json:"matches"`
}

// grypeMatch represents a single vulnerability match in Grype output.
type grypeMatch struct {
	Vulnerability grypeVuln     `json:"vulnerability"`
	Artifact      grypeArtifact `json:"artifact"`
}

// grypeVuln represents a single vulnerability in Grype output.
type grypeVuln struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Description string   `json:"description"`
	Fix         grypeFix `json:"fix"`
}

// grypeFix holds fix information for a Grype vulnerability.
type grypeFix struct {
	State    string   `json:"state"`
	Versions []string `json:"versions"`
}

// grypeArtifact represents the affected package in a Grype match.
type grypeArtifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// vulnRecord is a normalized, scanner-agnostic vulnerability row produced by the
// parse step and consumed by postReportToAPI. All scanner-specific output types
// (trivyVuln, grypeMatch, …) are converted into this shape before leaving the
// parsing layer — nothing outside the parse functions should reference raw
// scanner structs.
type vulnRecord struct {
	Image            string
	Target           string
	Library          string
	VulnerabilityID  string
	Severity         string
	Status           string
	InstalledVersion string
	FixedVersion     string
	Title            string
}

// ImageScanJobReconciler reconciles an ImageScanJob object.
type ImageScanJobReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset kubernetes.Interface

	// PollInterval is how often to requeue while a scan job is still running.
	// Controlled by BSO_POLL_INTERVAL env var (default: 15s).
	PollInterval time.Duration

	// TrivyImage is the container image used for Trivy scans.
	// Controlled by BSO_TRIVY_IMAGE env var (default: aquasec/trivy:0.58.1).
	TrivyImage string

	// GrypeImage is the container image used for Grype scans.
	// Controlled by BSO_GRYPE_IMAGE env var (default: anchore/grype:v0.90.0).
	GrypeImage string

	// MaxConcurrentScans caps the number of simultaneously active scan batch Jobs.
	// When the limit is reached, new Jobs are deferred until a slot opens.
	// 0 means unlimited. Controlled by BSO_MAX_CONCURRENT_SCANS env var.
	MaxConcurrentScans int

	// RegistryPullSecret is the name of a kubernetes.io/dockerconfigjson Secret
	// in the scan target namespace. When set, it is mounted into Trivy/Grype pods
	// as DOCKER_CONFIG so they can pull images from authenticated registries.
	// Controlled by BSO_REGISTRY_PULL_SECRET env var.
	RegistryPullSecret string

	// RegistryRewriteSource is the registry hostname used in pod image specs
	// (e.g. "BSO_REGISTRY_REWRITE_SOURCE.company_name.com"). When an image's registry prefix matches
	// this value it is replaced with RegistryRewriteTarget before passing the
	// image reference to Trivy/Grype.
	//
	// Use this when pods pull from a direct Nexus hosted/proxy repo address that
	// is different from the Docker group address that accepts your credentials.
	// Controlled by BSO_REGISTRY_REWRITE_SOURCE env var.
	RegistryRewriteSource string

	// RegistryRewriteTarget is the replacement registry hostname (e.g.
	// "registry.company_name.com"). Only used when RegistryRewriteSource is set.
	// Controlled by BSO_REGISTRY_REWRITE_TARGET env var.
	RegistryRewriteTarget string
}

// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=imagescanjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=imagescanjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=imagescanjobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

// Reconcile drives the lifecycle of an ImageScanJob.
func (r *ImageScanJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	isj := &securityv1alpha1.ImageScanJob{}
	if err := r.Get(ctx, req.NamespacedName, isj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Terminal states — nothing more to do.
	if isj.Status.Phase == securityv1alpha1.ImageScanJobPhaseCompleted ||
		isj.Status.Phase == securityv1alpha1.ImageScanJobPhaseFailed {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling ImageScanJob", "name", isj.Name, "image", isj.Spec.Image,
		"scanner", isj.Spec.Scanner, "phase", isj.Status.Phase)

	// Look for an existing Kubernetes Job owned by this ImageScanJob.
	kJob, err := r.findOwnedJob(ctx, isj)
	if err != nil {
		return ctrl.Result{}, err
	}

	// No Job yet — create one based on the configured scanner.
	if kJob == nil {
		// Enforce concurrency cap: defer job creation if too many are already running.
		if r.MaxConcurrentScans > 0 {
			active, err := r.countActiveJobs(ctx, isj.Namespace)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("counting active jobs: %w", err)
			}
			if active >= r.MaxConcurrentScans {
				log.Info("Concurrency limit reached, requeueing", "limit", r.MaxConcurrentScans, "active", active)
				return ctrl.Result{RequeueAfter: r.PollInterval}, nil
			}
		}

		if err := r.createScanJob(ctx, isj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create scan job (scanner=%s): %w", isj.Spec.Scanner, err)
		}
		msg := fmt.Sprintf("%s scan job created", isj.Spec.Scanner)
		if err := r.setPhase(ctx, isj, securityv1alpha1.ImageScanJobPhaseRunning, msg); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.PollInterval}, nil
	}

	// Job exists — check its state.
	switch {
	case isJobSucceeded(kJob):
		log.Info("Scan job succeeded, parsing results", "job", kJob.Name, "scanner", isj.Spec.Scanner)
		counts, records, parseErr := r.parseScanResults(ctx, isj, kJob)
		if parseErr != nil {
			log.Error(parseErr, "Failed to parse scan output; marking as failed")
			return ctrl.Result{}, r.markFailed(ctx, isj, kJob.Name, parseErr.Error())
		}
		if cmErr := r.postReportToAPI(ctx, isj, records); cmErr != nil {
			log.Error(cmErr, "Failed to POST scan report to API; continuing")
		}
		return ctrl.Result{}, r.markCompleted(ctx, isj, kJob.Name, counts)

	case isJobFailed(kJob):
		msg := jobFailureMessage(kJob)
		log.Info("Scan job failed", "job", kJob.Name, "reason", msg)
		return ctrl.Result{}, r.markFailed(ctx, isj, kJob.Name, msg)

	default:
		// Job is still running.
		if isj.Status.Phase != securityv1alpha1.ImageScanJobPhaseRunning {
			msg := fmt.Sprintf("Waiting for %s scan job to complete", isj.Spec.Scanner)
			if err := r.setPhase(ctx, isj, securityv1alpha1.ImageScanJobPhaseRunning, msg); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: r.PollInterval}, nil
	}
}

// findOwnedJob returns the Kubernetes Job owned by the given ImageScanJob, or nil if none exists.
// The job name is suffixed with the scanner name (e.g. "-trivy" or "-grype").
func (r *ImageScanJobReconciler) findOwnedJob(ctx context.Context, isj *securityv1alpha1.ImageScanJob) (*batchv1.Job, error) {
	jobName := isj.Name + scannerJobSuffix(isj.Spec.Scanner)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: isj.Namespace}, job)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// countActiveJobs returns the number of operator-managed batch Jobs that are
// currently pending or running (i.e. not yet terminal) in the given namespace.
func (r *ImageScanJobReconciler) countActiveJobs(ctx context.Context, namespace string) (int, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app.kubernetes.io/managed-by": "korsair-operator"},
	); err != nil {
		return 0, err
	}
	count := 0
	for _, job := range jobList.Items {
		if job.Status.Active > 0 || (job.Status.Succeeded == 0 && job.Status.Failed == 0) {
			count++
		}
	}
	return count, nil
}

// createScanJob dispatches to the scanner-specific job creation method.
func (r *ImageScanJobReconciler) createScanJob(ctx context.Context, isj *securityv1alpha1.ImageScanJob) error {
	switch isj.Spec.Scanner {
	case scannerGrype:
		return r.createGrypeJob(ctx, isj)
	default:
		return r.createTrivyJob(ctx, isj)
	}
}

// createTrivyJob creates a batch/v1 Job that runs Trivy against the target image.
func (r *ImageScanJobReconciler) createTrivyJob(ctx context.Context, isj *securityv1alpha1.ImageScanJob) error {
	log := logf.FromContext(ctx)

	// Rewrite the registry host if a source→target mapping is configured.
	// This lets Trivy authenticate against registry.company_name.com (Docker group)
	// even when pods reference images via BSO_REGISTRY_REWRITE_SOURCE.company_name.com.
	scanImage := rewriteRegistry(isj.Spec.Image, r.RegistryRewriteSource, r.RegistryRewriteTarget)
	if scanImage != isj.Spec.Image {
		log.Info("Registry rewrite applied", "original", isj.Spec.Image, "scanImage", scanImage)
	}

	jobName := isj.Name + trivyJobSuffix
	ttl := int32(3600)
	backoffLimit := int32(1)

	env := []corev1.EnvVar{
		{Name: "TRIVY_INSECURE", Value: "true"},
	}
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	if r.RegistryPullSecret != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "docker-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.RegistryPullSecret,
					Items: []corev1.KeyToPath{
						{Key: ".dockerconfigjson", Path: "config.json"},
					},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "docker-config",
			MountPath: "/root/.docker",
			ReadOnly:  true,
		})
		env = append(env, corev1.EnvVar{Name: "DOCKER_CONFIG", Value: "/root/.docker"})
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: isj.Namespace,
			Labels: map[string]string{
				"security.blacksyrius.com/imagescanresult": isj.Name,
				"security.blacksyrius.com/scanner":         "trivy",
				"app.kubernetes.io/managed-by":             "korsair-operator",
			},
			Annotations: map[string]string{
				"security.blacksyrius.com/image": isj.Spec.Image,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"security.blacksyrius.com/imagescanresult": isj.Name,
						"app.kubernetes.io/managed-by":             "korsair-operator",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       volumes,
					Containers: []corev1.Container{
						{
							Name:  "trivy",
							Image: r.TrivyImage,
							Args: []string{
								"image",
								"--format", "json",
								"--exit-code", "0",
								"--no-progress",
								"--timeout", "10m",
								scanImage,
							},
							Env:          env,
							VolumeMounts: volumeMounts,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: *mustParseQuantity("512Mi"),
									corev1.ResourceCPU:    *mustParseQuantity("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: *mustParseQuantity("2Gi"),
									corev1.ResourceCPU:    *mustParseQuantity("1"),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(isj, job, r.Scheme); err != nil {
		return err
	}

	log.Info("Creating Trivy job", "job", jobName, "image", isj.Spec.Image, "scanImage", scanImage)
	return r.Create(ctx, job)
}

// createGrypeJob creates a batch/v1 Job that runs Grype against the target image.
func (r *ImageScanJobReconciler) createGrypeJob(ctx context.Context, isj *securityv1alpha1.ImageScanJob) error {
	log := logf.FromContext(ctx)

	scanImage := rewriteRegistry(isj.Spec.Image, r.RegistryRewriteSource, r.RegistryRewriteTarget)
	if scanImage != isj.Spec.Image {
		log.Info("Registry rewrite applied", "original", isj.Spec.Image, "scanImage", scanImage)
	}

	jobName := isj.Name + grypeJobSuffix
	ttl := int32(3600)
	backoffLimit := int32(1)

	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	if r.RegistryPullSecret != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "docker-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.RegistryPullSecret,
					Items: []corev1.KeyToPath{
						{Key: ".dockerconfigjson", Path: "config.json"},
					},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "docker-config",
			MountPath: "/root/.docker",
			ReadOnly:  true,
		})
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: isj.Namespace,
			Labels: map[string]string{
				"security.blacksyrius.com/imagescanresult": isj.Name,
				"security.blacksyrius.com/scanner":         "grype",
				"app.kubernetes.io/managed-by":             "korsair-operator",
			},
			Annotations: map[string]string{
				"security.blacksyrius.com/image": isj.Spec.Image,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"security.blacksyrius.com/imagescanresult": isj.Name,
						"app.kubernetes.io/managed-by":             "korsair-operator",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       volumes,
					Containers: []corev1.Container{
						{
							Name:  "grype",
							Image: r.GrypeImage,
							Args: []string{
								scanImage,
								"-o", "json",
								"--quiet", // suppress structured JSON logs emitted to stderr in non-TTY pods
							},
							VolumeMounts: volumeMounts,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: *mustParseQuantity("512Mi"),
									corev1.ResourceCPU:    *mustParseQuantity("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: *mustParseQuantity("2Gi"),
									corev1.ResourceCPU:    *mustParseQuantity("1"),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(isj, job, r.Scheme); err != nil {
		return err
	}

	log.Info("Creating Grype job", "job", jobName, "image", isj.Spec.Image, "scanImage", scanImage)
	return r.Create(ctx, job)
}

// parseScanResults dispatches to the scanner-specific result parser.
func (r *ImageScanJobReconciler) parseScanResults(ctx context.Context, isj *securityv1alpha1.ImageScanJob, job *batchv1.Job) (map[string]int, []vulnRecord, error) {
	switch isj.Spec.Scanner {
	case "grype":
		return r.parseGrypeResults(ctx, isj.Spec.Image, job)
	default:
		return r.parseTrivyResults(ctx, isj.Spec.Image, job)
	}
}

// parseTrivyResults reads the completed Job pod's logs and returns CVE severity counts
// plus a full list of vulnerability records for CSV export.
func (r *ImageScanJobReconciler) parseTrivyResults(ctx context.Context, image string, job *batchv1.Job) (map[string]int, []vulnRecord, error) {
	data, err := r.readPodLogs(ctx, job, "trivy")
	if err != nil {
		return nil, nil, err
	}
	return parseTrivyJSON(image, data)
}

// parseGrypeResults reads the completed Grype Job pod's logs and returns CVE severity counts
// plus a full list of vulnerability records for CSV export.
func (r *ImageScanJobReconciler) parseGrypeResults(ctx context.Context, image string, job *batchv1.Job) (map[string]int, []vulnRecord, error) {
	data, err := r.readPodLogs(ctx, job, "grype")
	if err != nil {
		return nil, nil, err
	}
	return parseGrypeJSON(image, data)
}

// readPodLogs reads container logs from the first pod of the given Job.
func (r *ImageScanJobReconciler) readPodLogs(ctx context.Context, job *batchv1.Job, containerName string) ([]byte, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(job.Namespace),
		client.MatchingLabels{"job-name": job.Name},
	); err != nil {
		return nil, fmt.Errorf("listing job pods: %w", err)
	}
	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pods found for job %s", job.Name)
	}

	pod := podList.Items[0]
	logReq := r.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: containerName,
	})
	stream, err := logReq.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening log stream for pod %s: %w", pod.Name, err)
	}
	defer func() { _ = stream.Close() }()

	data, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("reading log stream: %w", err)
	}
	return data, nil
}

// parseTrivyJSON extracts CVE severity counts and full vulnerability records from raw Trivy JSON output.
// Trivy may emit informational lines before the JSON object; we search for the first '{'.
func parseTrivyJSON(image string, data []byte) (map[string]int, []vulnRecord, error) {
	raw := string(data)
	idx := strings.Index(raw, "{")
	if idx < 0 {
		return nil, nil, fmt.Errorf("no JSON object found in Trivy output (got %d bytes)", len(data))
	}

	var out trivyOutput
	if err := json.Unmarshal([]byte(raw[idx:]), &out); err != nil {
		return nil, nil, fmt.Errorf("parsing Trivy JSON: %w", err)
	}

	counts := map[string]int{"CRITICAL": 0, "HIGH": 0, "MEDIUM": 0, "LOW": 0}
	var records []vulnRecord
	for _, result := range out.Results {
		for _, vuln := range result.Vulnerabilities {
			severity := strings.ToUpper(vuln.Severity)
			if _, ok := counts[severity]; ok {
				counts[severity]++
			}
			records = append(records, vulnRecord{
				Image:            image,
				Target:           result.Target,
				Library:          vuln.PkgName,
				VulnerabilityID:  vuln.VulnerabilityID,
				Severity:         severity,
				Status:           vuln.Status,
				InstalledVersion: vuln.InstalledVersion,
				FixedVersion:     vuln.FixedVersion,
				Title:            vuln.Title,
			})
		}
	}
	return counts, records, nil
}

// parseGrypeJSON extracts CVE severity counts and full vulnerability records from raw Grype JSON output.
//
// Grype in non-TTY environments (Kubernetes pods) emits JSON-formatted structured log entries to
// stderr, which Kubernetes combines with stdout in pod logs. Naively searching for the first '{'
// would pick up a log entry (e.g. {"level":"info",...}) instead of the scan result, causing the
// parser to silently return 0 vulnerabilities (the log entry decodes fine but has no "matches").
//
// We instead search for the first '"matches"' key — which uniquely identifies Grype's output JSON —
// and then backtrack to the enclosing '{' to find the correct parse start.
func parseGrypeJSON(image string, data []byte) (map[string]int, []vulnRecord, error) {
	raw := string(data)

	// Find the "matches" key that uniquely identifies Grype's output object.
	before, _, ok := strings.Cut(raw, `"matches"`)
	if !ok {
		return nil, nil, fmt.Errorf("no Grype output found in pod logs (got %d bytes; expected JSON with \"matches\" key)", len(data))
	}
	// Backtrack to the '{' that opens the containing JSON object.
	idx := strings.LastIndex(before, "{")
	if idx < 0 {
		return nil, nil, fmt.Errorf("malformed Grype output: found \"matches\" key but no opening '{'")
	}

	var out grypeOutput
	if err := json.Unmarshal([]byte(raw[idx:]), &out); err != nil {
		return nil, nil, fmt.Errorf("parsing Grype JSON: %w", err)
	}

	counts := map[string]int{"CRITICAL": 0, "HIGH": 0, "MEDIUM": 0, "LOW": 0}
	var records []vulnRecord
	for _, match := range out.Matches {
		severity := strings.ToUpper(match.Vulnerability.Severity)
		if _, ok := counts[severity]; ok {
			counts[severity]++
		}

		fixedVersion := ""
		if len(match.Vulnerability.Fix.Versions) > 0 {
			fixedVersion = match.Vulnerability.Fix.Versions[0]
		}
		status := match.Vulnerability.Fix.State

		records = append(records, vulnRecord{
			Image:            image,
			Target:           match.Artifact.Name,
			Library:          match.Artifact.Name,
			VulnerabilityID:  match.Vulnerability.ID,
			Severity:         severity,
			Status:           status,
			InstalledVersion: match.Artifact.Version,
			FixedVersion:     fixedVersion,
			Title:            match.Vulnerability.Description,
		})
	}
	return counts, records, nil
}

// reportRow mirrors the JSON row shape accepted by the API's
// PUT /api/v1/jobs/{ns}/{name}/report endpoint. Field names must stay in sync
// with the API's reportRow struct in cmd/web/db.go — this is a wire format.
type reportRow struct {
	Image            string `json:"image"`
	Target           string `json:"target"`
	Library          string `json:"library"`
	VulnerabilityID  string `json:"vulnerabilityId"`
	Severity         string `json:"severity"`
	Status           string `json:"status"`
	InstalledVersion string `json:"installedVersion"`
	FixedVersion     string `json:"fixedVersion"`
	Title            string `json:"title"`
}

// reportWriteRequest is the top-level JSON body sent to the API's report endpoint.
// Scanner identifies the tool (trivy, grype) so the API can record provenance.
type reportWriteRequest struct {
	Scanner string      `json:"scanner"`
	Rows    []reportRow `json:"rows"`
}

// vulnToRow converts a scanner-agnostic vulnRecord to the API wire format.
func vulnToRow(rec vulnRecord) reportRow {
	return reportRow(rec)
}

var reportHTTPClient = &http.Client{Timeout: 30 * time.Second}

// postReportToAPI sends the parsed CVE rows to the korsair API server which
// persists them to PostgreSQL. Replaces the previous per-ImageScanJob
// ConfigMap that bloated etcd.
func (r *ImageScanJobReconciler) postReportToAPI(ctx context.Context, isj *securityv1alpha1.ImageScanJob, records []vulnRecord) error {
	log := logf.FromContext(ctx)

	apiURL := os.Getenv("KORSAIR_API_URL")
	if apiURL == "" {
		return fmt.Errorf("KORSAIR_API_URL not set")
	}

	rows := make([]reportRow, 0, len(records))
	for _, rec := range records {
		rows = append(rows, vulnToRow(rec))
	}

	body, err := json.Marshal(reportWriteRequest{Scanner: isj.Spec.Scanner, Rows: rows})
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/v1/jobs/%s/%s/report", strings.TrimRight(apiURL, "/"), isj.Namespace, isj.Name)
	token := os.Getenv("KORSAIR_INTERNAL_TOKEN")

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-Korsair-Token", token)
		}

		resp, err := reportHTTPClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				log.Info("Report POSTed to API", "job", isj.Name, "rows", len(rows), "attempt", attempt)
				return nil
			}
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
		}

		backoff := time.Duration(1<<attempt) * time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return fmt.Errorf("post report after retries: %w", lastErr)
}

// markCompleted updates the ImageScanJob status to Completed with CVE counts.
func (r *ImageScanJobReconciler) markCompleted(ctx context.Context, isj *securityv1alpha1.ImageScanJob, jobName string, counts map[string]int) error {
	base := isj.DeepCopy()
	now := metav1.Now()
	isj.Status.Phase = securityv1alpha1.ImageScanJobPhaseCompleted
	isj.Status.JobName = jobName
	isj.Status.CompletionTime = &now
	isj.Status.CriticalCount = counts["CRITICAL"]
	isj.Status.HighCount = counts["HIGH"]
	isj.Status.MediumCount = counts["MEDIUM"]
	isj.Status.LowCount = counts["LOW"]
	isj.Status.Message = fmt.Sprintf("Scan complete. Critical: %d, High: %d, Medium: %d, Low: %d",
		counts["CRITICAL"], counts["HIGH"], counts["MEDIUM"], counts["LOW"])

	apimeta.SetStatusCondition(&isj.Status.Conditions, metav1.Condition{
		Type:               "Complete",
		Status:             metav1.ConditionTrue,
		Reason:             "ScanSucceeded",
		ObservedGeneration: isj.Generation,
		Message:            isj.Status.Message,
	})
	return r.Status().Patch(ctx, isj, client.MergeFrom(base))
}

// markFailed updates the ImageScanJob status to Failed.
func (r *ImageScanJobReconciler) markFailed(ctx context.Context, isj *securityv1alpha1.ImageScanJob, jobName, reason string) error {
	base := isj.DeepCopy()
	isj.Status.Phase = securityv1alpha1.ImageScanJobPhaseFailed
	isj.Status.JobName = jobName
	isj.Status.Message = reason

	apimeta.SetStatusCondition(&isj.Status.Conditions, metav1.Condition{
		Type:               "Complete",
		Status:             metav1.ConditionFalse,
		Reason:             "ScanFailed",
		ObservedGeneration: isj.Generation,
		Message:            reason,
	})
	return r.Status().Patch(ctx, isj, client.MergeFrom(base))
}

// setPhase updates only the Phase and Message fields in the status.
func (r *ImageScanJobReconciler) setPhase(ctx context.Context, isj *securityv1alpha1.ImageScanJob, phase securityv1alpha1.ImageScanJobPhase, msg string) error {
	base := isj.DeepCopy()
	if isj.Status.Phase == "" {
		now := metav1.Now()
		isj.Status.StartTime = &now
	}
	isj.Status.Phase = phase
	isj.Status.Message = msg
	return r.Status().Patch(ctx, isj, client.MergeFrom(base))
}

// isJobSucceeded returns true when all Job completions have finished successfully.
func isJobSucceeded(job *batchv1.Job) bool {
	return job.Status.Succeeded > 0 && job.Status.Active == 0
}

// isJobFailed returns true when the Job has exhausted its backoff retries.
func isJobFailed(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// jobFailureMessage extracts a human-readable reason from Job conditions.
func jobFailureMessage(job *batchv1.Job) string {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed {
			return cond.Message
		}
	}
	return "job failed with unknown reason"
}

// SetupWithManager registers the controller with the Manager and watches owned Jobs.
func (r *ImageScanJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.ImageScanJob{}).
		Owns(&batchv1.Job{}).
		Named("imagescanresult").
		Complete(r)
}
