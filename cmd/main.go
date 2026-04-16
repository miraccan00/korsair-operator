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

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"k8s.io/client-go/kubernetes"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
	"github.com/miraccan00/korsair-operator/internal/controller"
	bsoslack "github.com/miraccan00/korsair-operator/internal/slack"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(securityv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Disable HTTP/2 by default to avoid stream cancellation CVEs.
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{TLSOpts: webhookTLSOpts}
	if len(webhookCertPath) > 0 {
		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}
	webhookServer := webhook.NewServer(webhookServerOptions)

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	if len(metricsCertPath) > 0 {
		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	restConfig := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "bso.security.blacksyrius.com",
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	// Build a raw kubernetes clientset for pod log access (used by ImageScanJobReconciler).
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		setupLog.Error(err, "Failed to create kubernetes clientset")
		os.Exit(1)
	}

	// Read configurable intervals from environment (set via .env for local dev).
	discoveryInterval := getEnvDuration("BSO_DISCOVERY_INTERVAL", 5*time.Minute)
	pollInterval := getEnvDuration("BSO_POLL_INTERVAL", 15*time.Second)
	notificationCooldown := getEnvDuration("BSO_NOTIFICATION_COOLDOWN", 1*time.Hour)

	setupLog.Info("BSO intervals configured",
		"discoveryInterval", discoveryInterval,
		"pollInterval", pollInterval,
		"notificationCooldown", notificationCooldown,
	)

	// Build optional Slack client from env var.
	slackClient := bsoslack.NewClient(os.Getenv("BSO_SLACK_WEBHOOK_URL"))

	// Build the list of globally enabled scanners from env flags.
	// BSO_TRIVY_ENABLED defaults to true; BSO_GRYPE_ENABLED defaults to false.
	trivyEnabled := getEnvBool("BSO_TRIVY_ENABLED", true)
	grypeEnabled := getEnvBool("BSO_GRYPE_ENABLED", false)
	trivyImg := getEnvString("BSO_TRIVY_IMAGE", "aquasec/trivy:0.58.1")
	grypeImg := getEnvString("BSO_GRYPE_IMAGE", "anchore/grype:v0.90.0")

	var enabledScanners []string
	if trivyEnabled {
		enabledScanners = append(enabledScanners, "trivy")
	}
	if grypeEnabled {
		enabledScanners = append(enabledScanners, "grype")
	}
	setupLog.Info("Scanner configuration",
		"enabledScanners", strings.Join(enabledScanners, ","),
		"trivyImage", trivyImg,
		"grypeImage", grypeImg,
	)

	// BSO_ENVIRONMENT controls environment-specific defaults ("test" or "prod").
	// In "test" mode, BSO_MAX_DISCOVERY_IMAGES defaults to 50 to avoid filling
	// local disk with hundreds of parallel Trivy jobs.
	environment := getEnvString("BSO_ENVIRONMENT", "prod")
	maxDiscoveryImages := getEnvInt("BSO_MAX_DISCOVERY_IMAGES", 0)
	if strings.EqualFold(environment, "test") && maxDiscoveryImages == 0 {
		maxDiscoveryImages = 50
	}
	maxConcurrentScans := getEnvInt("BSO_MAX_CONCURRENT_SCANS", 0)
	setupLog.Info("Environment configuration",
		"environment", environment,
		"maxDiscoveryImages", maxDiscoveryImages,
		"maxConcurrentScans", maxConcurrentScans,
	)

	// Register SecurityScanConfig controller.
	if err := (&controller.SecurityScanConfigReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		SlackClient:        slackClient,
		DiscoveryInterval:  discoveryInterval,
		DefaultCooldown:    notificationCooldown,
		EnabledScanners:    enabledScanners,
		MaxDiscoveryImages: maxDiscoveryImages,
		MaxConcurrentScans: maxConcurrentScans,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "SecurityScanConfig")
		os.Exit(1)
	}

	registryPullSecret := getEnvString("BSO_REGISTRY_PULL_SECRET", "")
	// Registry rewrite: when pod image specs use a different registry host than
	// the one that accepts credentials (e.g. BSO_REGISTRY_REWRITE_SOURCE.company_name.com vs
	// registry.company_name.com), set SOURCE→TARGET to rewrite before scanning.
	registryRewriteSource := getEnvString("BSO_REGISTRY_REWRITE_SOURCE", "")
	registryRewriteTarget := getEnvString("BSO_REGISTRY_REWRITE_TARGET", "")
	setupLog.Info("Scan job configuration",
		"maxConcurrentScans", maxConcurrentScans,
		"registryPullSecret", registryPullSecret,
		"registryRewriteSource", registryRewriteSource,
		"registryRewriteTarget", registryRewriteTarget,
	)

	// Register ImageScanJob controller.
	if err := (&controller.ImageScanJobReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		Clientset:             clientset,
		PollInterval:          pollInterval,
		TrivyImage:            trivyImg,
		GrypeImage:            grypeImg,
		MaxConcurrentScans:    maxConcurrentScans,
		RegistryPullSecret:    registryPullSecret,
		RegistryRewriteSource: registryRewriteSource,
		RegistryRewriteTarget: registryRewriteTarget,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "ImageScanJob")
		os.Exit(1)
	}

	// Register NotificationPolicy controller.
	if err := (&controller.NotificationPolicyReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "NotificationPolicy")
		os.Exit(1)
	}

	// Register ClusterTarget controller.
	if err := (&controller.ClusterTargetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "ClusterTarget")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	// Register scan job cleaner — periodically deletes old completed/failed ImageScanJob CRs.
	cleanupInterval := getEnvDuration("BSO_CLEANUP_INTERVAL", 5*time.Minute)
	scanJobRetention := getEnvDuration("BSO_SCAN_JOB_RETENTION", 1*time.Hour)
	setupLog.Info("Scan job cleaner configuration",
		"cleanupInterval", cleanupInterval,
		"scanJobRetention", scanJobRetention,
	)
	if err := mgr.Add(&controller.ScanJobCleaner{
		Client:    mgr.GetClient(),
		Interval:  cleanupInterval,
		Retention: scanJobRetention,
	}); err != nil {
		setupLog.Error(err, "Failed to register ScanJobCleaner")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting BSO manager", "version", "v0.4.0")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}

// getEnvDuration reads a Go duration string from an environment variable.
// Returns def if the variable is unset or cannot be parsed.
func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// getEnvBool reads a boolean env var ("true"/"1" → true, anything else → false).
// Returns def if the variable is unset.
func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return strings.EqualFold(v, "true") || v == "1"
}

// getEnvInt reads an integer env var. Returns def if unset or cannot be parsed.
func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

// getEnvString reads a string env var. Returns def if unset or empty.
func getEnvString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
