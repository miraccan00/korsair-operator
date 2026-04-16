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
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
	bsoslack "github.com/miraccan00/korsair-operator/internal/slack"
)

// operatorNamespace is the namespace where operator-managed resources
// (ClusterTargets, kubeconfig Secrets) live.
const operatorNamespace = "korsair-system"

// defaultMaxConcurrentScans is the fallback when BSO_MAX_CONCURRENT_SCANS is not set.
// scanBatchCooldown is the requeue delay when the concurrency limit is reached
// or when images are queued for the next batch.
const (
	defaultMaxConcurrentScans = 10
	scanBatchCooldown         = 2 * time.Minute
)

// notificationSettings holds resolved webhook and threshold configuration for a scan cycle.
type notificationSettings struct {
	WebhookURL     string
	Thresholds     *securityv1alpha1.ScanThresholds // nil = no threshold gate
	SeverityFilter []string
}

// SecurityScanConfigReconciler reconciles a SecurityScanConfig object.
type SecurityScanConfigReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	SlackClient *bsoslack.Client

	// DiscoveryInterval is how often to re-discover images and requeue.
	// Controlled by BSO_DISCOVERY_INTERVAL env var (default: 5m).
	// Used as fallback when spec.schedule is not set.
	DiscoveryInterval time.Duration

	// DefaultCooldown is the fallback notification cooldown when the CR spec
	// does not set notificationCooldown. Controlled by BSO_NOTIFICATION_COOLDOWN
	// env var (default: 1h).
	DefaultCooldown time.Duration

	// EnabledScanners is the global allow-list of scanners set via Helm values
	// (BSO_TRIVY_ENABLED / BSO_GRYPE_ENABLED). Only scanners that appear in
	// both this list and spec.scanners will be used. If empty, all scanners
	// listed in spec.scanners are allowed.
	EnabledScanners []string

	// MaxDiscoveryImages, when > 0, caps the number of unique images that will
	// receive ISJs per reconcile cycle. Images are selected deterministically
	// (sorted alphabetically) so the same subset is scanned every cycle.
	// Intended for test/dev environments to avoid exhausting local disk with
	// hundreds of parallel Trivy jobs. 0 = unlimited (production default).
	// Controlled by BSO_MAX_DISCOVERY_IMAGES env var.
	MaxDiscoveryImages int

	// MaxConcurrentScans caps the number of simultaneously Running/Pending ISJs.
	// 0 falls back to defaultMaxConcurrentScans (10).
	// Controlled by BSO_MAX_CONCURRENT_SCANS env var.
	MaxConcurrentScans int
}

// effectiveMaxConcurrentScans returns the configured limit or the default.
func (r *SecurityScanConfigReconciler) effectiveMaxConcurrentScans() int {
	if r.MaxConcurrentScans > 0 {
		return r.MaxConcurrentScans
	}
	return defaultMaxConcurrentScans
}

// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=securityscanconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=securityscanconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=securityscanconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=imagescanjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=notificationpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=clustertargets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;statefulsets,verbs=get;list;watch

// annotationReset is set to "true" to trigger a full reset of all ImageScanJobs
// owned by a SecurityScanConfig. The controller deletes all jobs and removes
// the annotation so the next reconcile cycle starts fresh.
const annotationReset = "security.blacksyrius.com/reset"

// Reconcile discovers images from configured sources and creates ImageScanJob resources.
func (r *SecurityScanConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	config := &securityv1alpha1.SecurityScanConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling SecurityScanConfig", "name", config.Name)

	// Handle reset annotation: wipe all ImageScanJobs for this config and start fresh.
	if config.Annotations[annotationReset] == "true" {
		if err := r.handleReset(ctx, config); err != nil {
			return ctrl.Result{}, fmt.Errorf("reset failed: %w", err)
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if !config.Spec.ImageSources.Kubernetes.Enabled {
		log.Info("Kubernetes image source is disabled, skipping discovery")
		return ctrl.Result{RequeueAfter: r.computeRequeue(config)}, nil
	}

	// Build namespace exclusion set.
	excludeNS := make(map[string]bool)
	for _, ns := range config.Spec.ImageSources.Kubernetes.ExcludeNamespaces {
		excludeNS[ns] = true
	}

	// Discover unique images from all running pods.
	images, err := r.discoverImages(ctx, excludeNS)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("image discovery failed: %w", err)
	}
	log.Info("Discovered images", "count", len(images))

	// Determine target namespace for ImageScanJob resources.
	targetNS := config.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = operatorNamespace
	}

	// Ensure target namespace exists.
	if err := r.ensureNamespace(ctx, targetNS); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure namespace %s: %w", targetNS, err)
	}

	// imageClusterMap tracks which ClusterTarget an image originates from.
	// Empty string means the image was discovered locally.
	imageClusterMap := make(map[string]string)

	// Build the list of ClusterTargets to fan-out to.
	// discoverAllClusters=true auto-selects every Connected ClusterTarget in the
	// operator namespace; otherwise only the explicitly listed refs are used.
	ctRefs := config.Spec.ImageSources.ClusterTargetRefs
	if config.Spec.ImageSources.DiscoverAllClusters {
		ctList := &securityv1alpha1.ClusterTargetList{}
		if err := r.List(ctx, ctList, client.InNamespace(operatorNamespace)); err != nil {
			log.Error(err, "Failed to list ClusterTargets for auto-discovery")
		} else {
			for _, ct := range ctList.Items {
				if ct.Status.Phase == securityv1alpha1.ClusterTargetPhaseConnected {
					ctRefs = append(ctRefs, ct.Name)
				}
			}
		}
	}

	// Fan-out image discovery to remote clusters.
	// ClusterTargets always live in korsair-system (operator namespace).
	for _, ctRef := range ctRefs {
		ct := &securityv1alpha1.ClusterTarget{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: operatorNamespace, Name: ctRef}, ct); err != nil {
			log.Error(err, "Failed to get ClusterTarget — skipping", "name", ctRef)
			continue
		}
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: ct.Namespace,
			Name:      ct.Spec.KubeconfigSecretRef.Name,
		}, secret); err != nil {
			log.Error(err, "Failed to get kubeconfig secret for ClusterTarget — skipping", "clusterTarget", ctRef)
			continue
		}
		remoteClient, err := BuildClientFromKubeconfigSecret(secret, r.Scheme)
		if err != nil {
			log.Error(err, "Failed to build remote client — skipping", "clusterTarget", ctRef)
			continue
		}
		remoteImages, err := discoverImagesFromClient(ctx, remoteClient, ct.Spec.ExcludeNamespaces)
		if err != nil {
			log.Error(err, "Remote image discovery failed — skipping", "clusterTarget", ctRef)
			continue
		}
		log.Info("Discovered remote images", "clusterTarget", ctRef, "count", len(remoteImages))
		for img, di := range remoteImages {
			if _, exists := images[img]; !exists {
				images[img] = di
			}
			if _, exists := imageClusterMap[img]; !exists {
				// First cluster that surfaces this image wins the label.
				imageClusterMap[img] = ctRef
			}
		}
	}

	// Determine which scanners to use (default to trivy).
	scanners := config.Spec.Scanners
	if len(scanners) == 0 {
		scanners = []string{"trivy"}
	}

	// Filter by the global enabled-scanners allow-list (from Helm values).
	if len(r.EnabledScanners) > 0 {
		scanners = intersectScanners(scanners, r.EnabledScanners)
	}
	if len(scanners) == 0 {
		log.Info("No enabled scanners remain after applying global scanner flags; skipping job creation")
		return ctrl.Result{RequeueAfter: r.computeRequeue(config)}, nil
	}

	// Pre-load all existing ImageScanJobs so we can skip the expensive resolveDigest
	// call for images that already have an active or completed scan job.
	// Only images whose ISJ is absent or Failed (and past the retry window) need fresh processing.
	existingISJs := &securityv1alpha1.ImageScanJobList{}
	if listErr := r.List(ctx, existingISJs, client.InNamespace(targetNS)); listErr != nil {
		return ctrl.Result{}, fmt.Errorf("listing existing ImageScanJobs: %w", listErr)
	}
	activeByImageScanner := make(map[string]bool) // key: image+"\x00"+scanner
	for _, isj := range existingISJs.Items {
		if isj.Status.Phase != securityv1alpha1.ImageScanJobPhaseFailed {
			activeByImageScanner[isj.Spec.Image+"\x00"+isj.Spec.Scanner] = true
		}
	}
	log.Info("Pre-loaded existing ImageScanJobs", "total", len(existingISJs.Items), "active", len(activeByImageScanner))

	// ── Batch scan throttle ──────────────────────────────────────────────────
	// Count currently active (Running/Pending/empty-phase) ISJs across the whole
	// target namespace so we never run more than r.effectiveMaxConcurrentScans() Trivy Jobs
	// simultaneously. This prevents resource exhaustion on smaller clusters.
	activeRunning := 0
	for _, isj := range existingISJs.Items {
		phase := isj.Status.Phase
		if phase != securityv1alpha1.ImageScanJobPhaseCompleted &&
			phase != securityv1alpha1.ImageScanJobPhaseFailed {
			activeRunning++
		}
	}
	capacity := r.effectiveMaxConcurrentScans() - activeRunning
	if capacity <= 0 {
		log.Info("Scan concurrency limit reached — waiting for slots to free up",
			"active", activeRunning, "limit", r.effectiveMaxConcurrentScans(), "requeueAfter", scanBatchCooldown)
		if err := r.updateStatus(ctx, config, len(images), targetNS); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
		return ctrl.Result{RequeueAfter: scanBatchCooldown}, nil
	}

	// Sort images by pod creation time ascending (oldest deployment first) for
	// deterministic batch ordering. When the cap is applied, the oldest N images
	// are always selected, prioritising long-running workloads.
	sortedImages := make([]string, 0, len(images))
	for img := range images {
		sortedImages = append(sortedImages, img)
	}
	sort.Slice(sortedImages, func(i, j int) bool {
		ti := images[sortedImages[i]].DeployedAt
		tj := images[sortedImages[j]].DeployedAt
		if ti.Equal(tj) {
			return sortedImages[i] < sortedImages[j] // stable tie-break
		}
		return ti.Before(tj)
	})

	// Apply optional discovery image cap (BSO_MAX_DISCOVERY_IMAGES).
	// When set, only the oldest N images are considered for scanning.
	if r.MaxDiscoveryImages > 0 && len(sortedImages) > r.MaxDiscoveryImages {
		log.Info("Discovery image limit applied — truncating scan candidates",
			"total", len(sortedImages), "limit", r.MaxDiscoveryImages)
		sortedImages = sortedImages[:r.MaxDiscoveryImages]
	}

	// Create ImageScanJob for each unique image × scanner combination, up to capacity.
	created := 0
	pendingForNextBatch := 0
	for _, image := range sortedImages {
		sourceNS := images[image].Namespace

		// Determine which scanners still need a job for this image.
		var needsScan []string
		for _, scanner := range scanners {
			if !activeByImageScanner[image+"\x00"+scanner] {
				needsScan = append(needsScan, scanner)
			}
		}
		if len(needsScan) == 0 {
			continue // all scanners have an active or completed job — skip expensive digest call
		}

		if created >= capacity {
			// Batch is full — count this image as pending for the next cycle.
			pendingForNextBatch++
			continue
		}

		digest, digestErr := resolveDigest(image, authn.DefaultKeychain)
		if digestErr != nil {
			log.Error(digestErr, "Could not resolve image digest, using name hash fallback — image may be local-only or registry inaccessible", "image", image)
		} else {
			log.Info("Resolved image digest", "image", image, "digest", digest[:19]) // sha256:xxxxxxxx
		}
		clusterTarget := imageClusterMap[image] // "" for locally discovered images
		for _, scanner := range needsScan {
			if created >= capacity {
				pendingForNextBatch++
				break
			}
			jobName := imageToJobName(image, scanner, digest)
			if err := r.ensureImageScanJob(ctx, jobName, targetNS, image, config.Name, scanner, digest, clusterTarget, sourceNS, images[image].DeployedAt); err != nil {
				log.Error(err, "Failed to ensure ImageScanJob", "image", image, "scanner", scanner)
				continue
			}
			created++
		}
	}
	if created > 0 {
		log.Info("Created new ImageScanJobs", "count", created, "pendingForNextBatch", pendingForNextBatch)
	}

	// Update status and maybe send Slack notification.
	if err := r.updateStatus(ctx, config, len(images), targetNS); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// If images are waiting for the next batch, requeue sooner than the normal
	// discovery interval so they get processed as scan slots free up.
	if pendingForNextBatch > 0 {
		log.Info("More images pending scan — requeueing for next batch",
			"pending", pendingForNextBatch, "cooldown", scanBatchCooldown)
		return ctrl.Result{RequeueAfter: scanBatchCooldown}, nil
	}

	return ctrl.Result{RequeueAfter: r.computeRequeue(config)}, nil
}

// computeRequeue determines the next reconcile delay.
// If spec.schedule is a valid cron expression, it schedules the next run accordingly.
// Otherwise it falls back to DiscoveryInterval.
func (r *SecurityScanConfigReconciler) computeRequeue(config *securityv1alpha1.SecurityScanConfig) time.Duration {
	if config.Spec.Schedule != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := parser.Parse(config.Spec.Schedule)
		if err == nil {
			next := schedule.Next(time.Now())
			if d := time.Until(next); d > 0 {
				return d
			}
		}
		// Invalid or past schedule — fall through to default.
	}
	return r.DiscoveryInterval
}

// resolveNotificationSettings looks up the effective webhook URL and thresholds.
// If notificationPolicyRef is set, the policy overrides the global defaults.
func (r *SecurityScanConfigReconciler) resolveNotificationSettings(ctx context.Context, config *securityv1alpha1.SecurityScanConfig) (*notificationSettings, error) {
	if config.Spec.NotificationPolicyRef == "" {
		url := ""
		if r.SlackClient != nil {
			url = r.SlackClient.WebhookURL
		}
		return &notificationSettings{WebhookURL: url}, nil
	}

	targetNS := config.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = operatorNamespace
	}

	var policy securityv1alpha1.NotificationPolicy
	if err := r.Get(ctx, types.NamespacedName{
		Name:      config.Spec.NotificationPolicyRef,
		Namespace: targetNS,
	}, &policy); err != nil {
		return nil, fmt.Errorf("getting NotificationPolicy %q: %w", config.Spec.NotificationPolicyRef, err)
	}

	settings := &notificationSettings{
		WebhookURL:     policy.Spec.WebhookURL,
		SeverityFilter: policy.Spec.SeverityFilter,
	}
	if policy.Spec.Thresholds != nil {
		settings.Thresholds = policy.Spec.Thresholds
	}
	return settings, nil
}

// exceedsThresholds returns true if any image result exceeds a configured severity threshold,
// or if no thresholds are defined (nil = always notify).
func exceedsThresholds(results []bsoslack.ImageResult, thresholds *securityv1alpha1.ScanThresholds) bool {
	if thresholds == nil {
		return true // no threshold gate — always notify
	}
	for _, r := range results {
		if r.CriticalCount > thresholds.Critical {
			return true
		}
		if r.HighCount > thresholds.High {
			return true
		}
	}
	return false
}

// discoveredImage holds metadata about a container image found in the cluster.
type discoveredImage struct {
	Namespace  string
	DeployedAt time.Time // creation time of the oldest pod running this image
}

// discoverImages lists all pods in the local cluster and collects unique container image references.
// Returns a map of image → discoveredImage, where DeployedAt is the earliest pod creation time seen.
func (r *SecurityScanConfigReconciler) discoverImages(ctx context.Context, excludeNS map[string]bool) (map[string]discoveredImage, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList); err != nil {
		return nil, err
	}

	images := make(map[string]discoveredImage)
	recordImage := func(img, ns string, podTime time.Time) {
		if img == "" {
			return
		}
		if existing, exists := images[img]; !exists || podTime.Before(existing.DeployedAt) {
			images[img] = discoveredImage{Namespace: ns, DeployedAt: podTime}
		}
	}
	for _, pod := range podList.Items {
		if excludeNS[pod.Namespace] {
			continue
		}
		t := pod.CreationTimestamp.Time
		for _, c := range pod.Spec.Containers {
			recordImage(c.Image, pod.Namespace, t)
		}
		for _, c := range pod.Spec.InitContainers {
			recordImage(c.Image, pod.Namespace, t)
		}
	}
	return images, nil
}

// discoverImagesFromClient lists pods via the given client (used for remote clusters)
// and returns unique container image references, excluding the specified namespaces.
// Returns a map of image → discoveredImage, where DeployedAt is the earliest pod creation time seen.
func discoverImagesFromClient(ctx context.Context, cl client.Client, excludeNamespaces []string) (map[string]discoveredImage, error) {
	excludeNS := make(map[string]bool, len(excludeNamespaces))
	for _, ns := range excludeNamespaces {
		excludeNS[ns] = true
	}

	podList := &corev1.PodList{}
	if err := cl.List(ctx, podList); err != nil {
		return nil, err
	}

	images := make(map[string]discoveredImage)
	recordImage := func(img, ns string, podTime time.Time) {
		if img == "" {
			return
		}
		if existing, exists := images[img]; !exists || podTime.Before(existing.DeployedAt) {
			images[img] = discoveredImage{Namespace: ns, DeployedAt: podTime}
		}
	}
	for _, pod := range podList.Items {
		if excludeNS[pod.Namespace] {
			continue
		}
		t := pod.CreationTimestamp.Time
		for _, c := range pod.Spec.Containers {
			recordImage(c.Image, pod.Namespace, t)
		}
		for _, c := range pod.Spec.InitContainers {
			recordImage(c.Image, pod.Namespace, t)
		}
	}
	return images, nil
}

// ensureNamespace creates the target namespace if it does not exist.
func (r *SecurityScanConfigReconciler) ensureNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: name}, ns)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "korsair-operator",
			},
		},
	}
	return r.Create(ctx, ns)
}

// ensureImageScanJob creates an ImageScanJob if it does not already exist.
// Returns nil (and does nothing) when the job already exists.
// digest is stored in Spec.ImageDigest when non-empty.
// clusterTarget, when non-empty, is added as a label to indicate the image source cluster.
// sourceNamespace is the namespace of the pod where the image was discovered.
func (r *SecurityScanConfigReconciler) ensureImageScanJob(ctx context.Context, jobName, namespace, image, configRef, scanner, digest, clusterTarget, sourceNamespace string, deployedAt time.Time) error {
	existing := &securityv1alpha1.ImageScanJob{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: namespace}, existing)
	if err == nil {
		// If the previous scan failed, retry it — but only after a cooldown to avoid
		// hammering the registry or cluster with back-to-back failures.
		if existing.Status.Phase == securityv1alpha1.ImageScanJobPhaseFailed {
			const failedRetryInterval = 2 * time.Hour
			if existing.Status.CompletionTime != nil &&
				time.Since(existing.Status.CompletionTime.Time) < failedRetryInterval {
				return nil // still within retry cooldown — skip
			}
			if delErr := r.Delete(ctx, existing); delErr != nil {
				return fmt.Errorf("deleting failed ImageScanJob %q for retry: %w", jobName, delErr)
			}
			// Fall through to recreate below.
		} else if existing.Spec.SourceNamespace == "" && sourceNamespace != "" {
			// Migration: backfill sourceNamespace on jobs created before this field existed.
			patch := client.MergeFrom(existing.DeepCopy())
			existing.Spec.SourceNamespace = sourceNamespace
			return r.Patch(ctx, existing, patch)
		} else {
			return nil // already exists and is up to date
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	labels := map[string]string{
		"security.blacksyrius.com/config":  configRef,
		"security.blacksyrius.com/scanner": scanner,
	}
	if clusterTarget != "" {
		labels["security.blacksyrius.com/cluster-target"] = clusterTarget
	}

	isj := &securityv1alpha1.ImageScanJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"security.blacksyrius.com/image":       image,
				"security.blacksyrius.com/deployed-at": deployedAt.UTC().Format(time.RFC3339),
			},
		},
		Spec: securityv1alpha1.ImageScanJobSpec{
			Image:           image,
			ImageDigest:     digest,
			Scanner:         scanner,
			Source:          "kubernetes",
			ConfigRef:       configRef,
			SourceNamespace: sourceNamespace,
		},
	}
	return r.Create(ctx, isj)
}

// handleReset deletes all ImageScanJobs belonging to this config and removes
// the reset annotation so the next reconcile cycle starts from scratch.
func (r *SecurityScanConfigReconciler) handleReset(ctx context.Context, config *securityv1alpha1.SecurityScanConfig) error {
	log := logf.FromContext(ctx)

	targetNS := config.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = operatorNamespace
	}

	var jobList securityv1alpha1.ImageScanJobList
	if err := r.List(ctx, &jobList,
		client.InNamespace(targetNS),
		client.MatchingLabels{"security.blacksyrius.com/config": config.Name},
	); err != nil {
		return fmt.Errorf("listing ImageScanJobs for reset: %w", err)
	}

	deleted := 0
	for i := range jobList.Items {
		if err := r.Delete(ctx, &jobList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete ImageScanJob during reset", "name", jobList.Items[i].Name)
		} else {
			deleted++
		}
	}
	log.Info("Reset complete — deleted ImageScanJobs", "config", config.Name, "deleted", deleted)

	// Remove the reset annotation so the next reconcile starts fresh.
	patch := client.MergeFrom(config.DeepCopy())
	delete(config.Annotations, annotationReset)
	return r.Patch(ctx, config, patch)
}

// updateStatus refreshes SecurityScanConfig.Status with current job counts and
// sends a Slack notification when all jobs are terminal and the cooldown has elapsed.
//
// Notification design:
//   - Fires only when ALL jobs are terminal AND at least one completed successfully.
//   - LastNotificationTime + NotifiedImageCount are persisted in the SAME Status().Update()
//     call as the regular status fields — this is the atomic update that prevents duplicate
//     notifications from a race condition between two separate Status().Update() calls.
//   - If notificationPolicyRef is set, the policy's webhook URL and thresholds are used.
func (r *SecurityScanConfigReconciler) updateStatus(ctx context.Context, config *securityv1alpha1.SecurityScanConfig, totalImages int, targetNS string) error {
	log := logf.FromContext(ctx)
	base := config.DeepCopy()

	isjList := &securityv1alpha1.ImageScanJobList{}
	if listErr := r.List(ctx, isjList,
		client.InNamespace(targetNS),
		client.MatchingLabels{"security.blacksyrius.com/config": config.Name},
	); listErr != nil {
		log.Error(listErr, "Failed to list ImageScanJobs for status update", "namespace", targetNS)
	}

	active, completed, failed := 0, 0, 0
	for _, j := range isjList.Items {
		switch j.Status.Phase {
		case securityv1alpha1.ImageScanJobPhaseCompleted:
			completed++
		case securityv1alpha1.ImageScanJobPhaseFailed:
			failed++
		default:
			active++
		}
	}

	now := metav1.Now()
	config.Status.LastScanTime = &now
	config.Status.TotalImages = totalImages
	config.Status.ActiveJobs = active
	config.Status.CompletedJobs = completed
	config.Status.FailedJobs = failed

	meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		ObservedGeneration: config.Generation,
		Message: fmt.Sprintf("Discovered %d images. Jobs: %d active, %d completed, %d failed",
			totalImages, active, completed, failed),
	})

	// --- Notification gate ---
	allTerminal := len(isjList.Items) > 0 && active == 0 && (completed+failed) > 0
	shouldNotify := allTerminal && completed > 0

	if shouldNotify {
		cooldown := r.effectiveCooldown(config.Spec.NotificationCooldown)
		withinCooldown := config.Status.LastNotificationTime != nil &&
			time.Since(config.Status.LastNotificationTime.Time) < cooldown
		newImagesSinceLast := completed+failed != config.Status.NotifiedImageCount

		if withinCooldown && !newImagesSinceLast {
			log.Info("Slack notification suppressed by cooldown",
				"cooldown", cooldown,
				"elapsed", time.Since(config.Status.LastNotificationTime.Time).Round(time.Second))
			shouldNotify = false
		}
	}

	// Resolve notification settings (webhook URL + thresholds).
	var settings *notificationSettings
	if shouldNotify {
		var settingsErr error
		settings, settingsErr = r.resolveNotificationSettings(ctx, config)
		if settingsErr != nil {
			log.Error(settingsErr, "Failed to resolve notification settings; skipping notification")
			shouldNotify = false
		}
	}

	// Apply threshold gate if a NotificationPolicy with thresholds is configured.
	if shouldNotify && settings != nil {
		summary := buildSlackSummary(config.Name, targetNS, isjList.Items)
		if !exceedsThresholds(summary.ImageResults, settings.Thresholds) {
			log.Info("Slack notification suppressed by threshold gate")
			shouldNotify = false
		}
	}

	// Check that we have a webhook URL to send to.
	if shouldNotify && (settings == nil || settings.WebhookURL == "") {
		log.Info("Slack notification skipped — no webhook URL configured (set BSO_SLACK_WEBHOOK_URL or use notificationPolicyRef)")
		shouldNotify = false
	}

	// Persist notification state atomically with the regular status fields.
	if shouldNotify {
		config.Status.LastNotificationTime = &now
		config.Status.NotifiedImageCount = completed + failed
	}

	// Single atomic status patch (merge patch avoids resource-version conflicts when
	// concurrent ISJ-completion events trigger multiple SSC reconciles simultaneously).
	if err := r.Status().Patch(ctx, config, client.MergeFrom(base)); err != nil {
		return err
	}

	if !shouldNotify {
		if allTerminal && completed == 0 && failed > 0 {
			log.Info("All scans failed — Slack notification skipped (no successful results)",
				"failed", failed)
		}
		return nil
	}

	slackClient := bsoslack.NewClient(settings.WebhookURL)
	summary := buildSlackSummary(config.Name, targetNS, isjList.Items)
	if err := slackClient.SendScanReport(summary); err != nil {
		log.Error(err, "Failed to send Slack notification")
		return nil // non-fatal
	}
	log.Info("Slack batch notification sent",
		"config", config.Name,
		"webhook", settings.WebhookURL[:min(len(settings.WebhookURL), 40)],
		"completed", completed,
		"failed", failed)
	return nil
}

// effectiveCooldown resolves the notification cooldown in priority order:
//  1. spec.notificationCooldown (per-CR override)
//  2. r.DefaultCooldown         (from BSO_NOTIFICATION_COOLDOWN env var)
//  3. 1h                        (hardcoded minimum)
func (r *SecurityScanConfigReconciler) effectiveCooldown(specCooldown string) time.Duration {
	if specCooldown != "" {
		if d, err := time.ParseDuration(specCooldown); err == nil && d > 0 {
			return d
		}
	}
	if r.DefaultCooldown > 0 {
		return r.DefaultCooldown
	}
	return 1 * time.Hour
}

// buildSlackSummary constructs the ScanSummary payload from the current ImageScanJob list.
func buildSlackSummary(configName, namespace string, jobs []securityv1alpha1.ImageScanJob) bsoslack.ScanSummary {
	results := make([]bsoslack.ImageResult, 0, len(jobs))
	for _, j := range jobs {
		results = append(results, bsoslack.ImageResult{
			Image:         j.Spec.Image,
			Scanner:       j.Spec.Scanner,
			Phase:         string(j.Status.Phase),
			CriticalCount: j.Status.CriticalCount,
			HighCount:     j.Status.HighCount,
			MediumCount:   j.Status.MediumCount,
			LowCount:      j.Status.LowCount,
			ReportCMName:  j.Name + "-report",
		})
	}
	return bsoslack.ScanSummary{
		ConfigName:   configName,
		Namespace:    namespace,
		ImageResults: results,
	}
}

// imageToJobName converts an image reference and scanner to a deterministic Kubernetes resource name.
// When a digest is available it uses the first 8 hex chars of the digest (after "sha256:" prefix)
// so that two tags pointing to the same image produce the same job name — dedup by content.
// Falls back to SHA256(imageString)[:8] when digest resolution failed.
func imageToJobName(image, scanner, digest string) string {
	if digest != "" && len(digest) > 15 {
		// digest = "sha256:09fb0c6289..." → skip 7-char prefix, take next 8 chars
		return fmt.Sprintf("scan-%s-%s", digest[7:15], scanner)
	}
	h := sha256.Sum256([]byte(image))
	return fmt.Sprintf("scan-%x-%s", h[:8], scanner)
}

// intersectScanners returns the elements of requested that are also present in allowed.
func intersectScanners(requested, allowed []string) []string {
	set := make(map[string]bool, len(allowed))
	for _, s := range allowed {
		set[s] = true
	}
	var result []string
	for _, s := range requested {
		if set[s] {
			result = append(result, s)
		}
	}
	return result
}

// resolveDigest fetches the content-addressable digest for an image tag using a lightweight
// HTTP HEAD request (no layers downloaded). Returns ("", err) on failure so callers can log
// the reason and fall back gracefully to name-hash based deduplication.
func resolveDigest(image string, kc authn.Keychain) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	digest, err := crane.Digest(image, crane.WithAuthFromKeychain(kc), crane.WithContext(ctx))
	if err != nil {
		return "", err
	}
	return digest, nil
}

// SetupWithManager registers the controller with the Manager.
// It also watches ImageScanJob objects so that job completions trigger an immediate
// reconcile of the parent SecurityScanConfig — event-driven, not poll-driven.
// Additionally watches ClusterTarget objects so that when a remote cluster becomes
// Connected, SecurityScanConfigs with discoverAllClusters:true are triggered immediately
// rather than waiting for the next scheduled discovery interval.
func (r *SecurityScanConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapISJToSSC := func(ctx context.Context, obj client.Object) []reconcile.Request {
		isj, ok := obj.(*securityv1alpha1.ImageScanJob)
		if !ok {
			return nil
		}
		configRef := isj.Spec.ConfigRef
		if configRef == "" {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{Name: configRef}},
		}
	}

	// When a ClusterTarget changes phase (e.g. becomes Connected after the initial probe),
	// immediately trigger all SecurityScanConfigs that use discoverAllClusters so they
	// pick up the newly available remote cluster without waiting for the next poll cycle.
	mapCTToSSCs := func(ctx context.Context, obj client.Object) []reconcile.Request {
		var sscList securityv1alpha1.SecurityScanConfigList
		if err := mgr.GetClient().List(ctx, &sscList); err != nil {
			return nil
		}
		var requests []reconcile.Request
		for _, ssc := range sscList.Items {
			if ssc.Spec.ImageSources.DiscoverAllClusters {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: ssc.Name},
				})
			}
		}
		return requests
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.SecurityScanConfig{}).
		Watches(
			&securityv1alpha1.ImageScanJob{},
			handler.EnqueueRequestsFromMapFunc(mapISJToSSC),
		).
		Watches(
			&securityv1alpha1.ClusterTarget{},
			handler.EnqueueRequestsFromMapFunc(mapCTToSSCs),
		).
		Named("securityscanconfig").
		Complete(r)
}
