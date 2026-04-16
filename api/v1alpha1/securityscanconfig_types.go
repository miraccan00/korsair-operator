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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageSourcesSpec defines which sources to discover images from.
type ImageSourcesSpec struct {
	// kubernetes enables image discovery from running pods/workloads in the cluster.
	// +optional
	Kubernetes KubernetesSource `json:"kubernetes,omitempty"`

	// clusterTargetRefs is a list of ClusterTarget names (in korsair-system) whose
	// remote clusters will also be scanned. Images discovered in remote clusters are merged
	// with locally discovered images and scanned using the same scanner pipeline.
	// +optional
	ClusterTargetRefs []string `json:"clusterTargetRefs,omitempty"`

	// discoverAllClusters, when true, automatically includes every ClusterTarget in
	// korsair-system that has Phase=Connected. This eliminates the need to manually
	// update clusterTargetRefs when new clusters are registered.
	// +optional
	DiscoverAllClusters bool `json:"discoverAllClusters,omitempty"`
}

// KubernetesSource configures image discovery from Kubernetes workloads.
type KubernetesSource struct {
	// enabled controls whether Kubernetes image discovery is active.
	// +required
	Enabled bool `json:"enabled"`

	// excludeNamespaces is a list of namespaces to skip during image discovery.
	// +optional
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`
}

// SecurityScanConfigSpec defines the desired state of SecurityScanConfig.
type SecurityScanConfigSpec struct {
	// imageSources defines which registries and cluster resources to discover images from.
	// +required
	ImageSources ImageSourcesSpec `json:"imageSources"`

	// schedule is a cron expression for periodic scanning (e.g. "0 2 * * *").
	// If empty, the operator will scan once on creation and not reschedule.
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// scanners lists which scanner tools to use. Defaults to ["trivy"].
	// +optional
	// +kubebuilder:default={"trivy"}
	Scanners []string `json:"scanners,omitempty"`

	// targetNamespace is the namespace where ImageScanJob resources will be created.
	// Defaults to korsair-system.
	// +optional
	// +kubebuilder:default="korsair-system"
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// notificationCooldown is the minimum duration between successive Slack notifications
	// for this SecurityScanConfig. Prevents alert spam when scan cycles run frequently.
	// Accepts Go duration strings: "30m", "1h", "6h", "24h".
	// Falls back to the BSO_NOTIFICATION_COOLDOWN env var, then defaults to "1h".
	// +optional
	NotificationCooldown string `json:"notificationCooldown,omitempty"`

	// notificationPolicyRef is the name of a NotificationPolicy in the target namespace.
	// When set, the webhook URL and thresholds from that policy are used instead of the
	// global BSO_SLACK_WEBHOOK_URL environment variable.
	// +optional
	NotificationPolicyRef string `json:"notificationPolicyRef,omitempty"`
}

// SecurityScanConfigStatus defines the observed state of SecurityScanConfig.
type SecurityScanConfigStatus struct {
	// conditions represent the current state of the SecurityScanConfig.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// lastScanTime is the timestamp of the last image discovery run.
	// +optional
	LastScanTime *metav1.Time `json:"lastScanTime,omitempty"`

	// totalImages is the number of unique images discovered in the last scan cycle.
	// +optional
	TotalImages int `json:"totalImages,omitempty"`

	// activeJobs is the number of ImageScanJobs currently running.
	// +optional
	ActiveJobs int `json:"activeJobs,omitempty"`

	// completedJobs is the number of ImageScanJobs that have finished successfully.
	// +optional
	CompletedJobs int `json:"completedJobs,omitempty"`

	// failedJobs is the number of ImageScanJobs that have failed.
	// +optional
	FailedJobs int `json:"failedJobs,omitempty"`

	// lastNotificationTime is the timestamp of the last successful Slack notification.
	// Used to enforce the notificationCooldown and survive operator restarts.
	// +optional
	LastNotificationTime *metav1.Time `json:"lastNotificationTime,omitempty"`

	// notifiedImageCount records how many images were in the batch when the last
	// notification was sent. If this changes (new images), notification is re-armed.
	// +optional
	NotifiedImageCount int `json:"notifiedImageCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ssc
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.schedule"
// +kubebuilder:printcolumn:name="TotalImages",type="integer",JSONPath=".status.totalImages"
// +kubebuilder:printcolumn:name="ActiveJobs",type="integer",JSONPath=".status.activeJobs"
// +kubebuilder:printcolumn:name="CompletedJobs",type="integer",JSONPath=".status.completedJobs"
// +kubebuilder:printcolumn:name="LastScan",type="date",JSONPath=".status.lastScanTime"

// SecurityScanConfig is the main operator configuration resource.
// It defines image sources, scanning schedule, and scanner tools to use.
type SecurityScanConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired scanning configuration.
	// +required
	Spec SecurityScanConfigSpec `json:"spec"`

	// status reflects the observed state of the scanning configuration.
	// +optional
	Status SecurityScanConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecurityScanConfigList contains a list of SecurityScanConfig.
type SecurityScanConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityScanConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecurityScanConfig{}, &SecurityScanConfigList{})
}
