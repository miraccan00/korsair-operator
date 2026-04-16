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

// ImageScanJobPhase represents the lifecycle phase of an ImageScanJob.
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
type ImageScanJobPhase string

const (
	// ImageScanJobPhasePending means the job is queued but no Kubernetes Job has been created yet.
	ImageScanJobPhasePending ImageScanJobPhase = "Pending"
	// ImageScanJobPhaseRunning means the Kubernetes Job is active.
	ImageScanJobPhaseRunning ImageScanJobPhase = "Running"
	// ImageScanJobPhaseCompleted means the scan finished successfully.
	ImageScanJobPhaseCompleted ImageScanJobPhase = "Completed"
	// ImageScanJobPhaseFailed means the scan job failed.
	ImageScanJobPhaseFailed ImageScanJobPhase = "Failed"
)

// ImageScanJobSpec defines the desired state of ImageScanJob.
// ImageScanJob resources are created automatically by the SecurityScanConfig controller.
type ImageScanJobSpec struct {
	// image is the full image reference to scan (e.g. docker.io/library/nginx:1.25).
	// +required
	Image string `json:"image"`

	// imageDigest is the content-addressable digest of the image.
	// Used for deduplication — if two tags point to the same digest, only one scan runs.
	// +optional
	ImageDigest string `json:"imageDigest,omitempty"`

	// scanner is the tool used for scanning. Supported values: "trivy", "grype".
	// +required
	// +kubebuilder:default="trivy"
	// +kubebuilder:validation:Enum=trivy;grype
	Scanner string `json:"scanner"`

	// source identifies where this image was discovered from.
	// +required
	// +kubebuilder:validation:Enum=kubernetes;manual
	Source string `json:"source"`

	// configRef is the name of the SecurityScanConfig that triggered this job.
	// +required
	ConfigRef string `json:"configRef"`

	// sourceNamespace is the Kubernetes namespace where the pod running this image was discovered.
	// This may differ from the namespace where the ImageScanJob itself is created (targetNamespace).
	// +optional
	SourceNamespace string `json:"sourceNamespace,omitempty"`
}

// ImageScanJobStatus defines the observed state of ImageScanJob.
type ImageScanJobStatus struct {
	// phase is the current lifecycle phase of this scan job.
	// +optional
	Phase ImageScanJobPhase `json:"phase,omitempty"`

	// jobName is the name of the underlying Kubernetes batch/v1 Job.
	// +optional
	JobName string `json:"jobName,omitempty"`

	// startTime is when the Kubernetes Job was created.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// completionTime is when the scan finished (success or failure).
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// criticalCount is the number of CRITICAL severity vulnerabilities found.
	// +optional
	CriticalCount int `json:"criticalCount,omitempty"`

	// highCount is the number of HIGH severity vulnerabilities found.
	// +optional
	HighCount int `json:"highCount,omitempty"`

	// mediumCount is the number of MEDIUM severity vulnerabilities found.
	// +optional
	MediumCount int `json:"mediumCount,omitempty"`

	// lowCount is the number of LOW severity vulnerabilities found.
	// +optional
	LowCount int `json:"lowCount,omitempty"`

	// message provides a human-readable status message or error description.
	// +optional
	Message string `json:"message,omitempty"`

	// conditions represent the detailed state of the ImageScanJob.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=isj
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image",priority=0
// +kubebuilder:printcolumn:name="Scanner",type="string",JSONPath=".spec.scanner"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Critical",type="integer",JSONPath=".status.criticalCount"
// +kubebuilder:printcolumn:name="High",type="integer",JSONPath=".status.highCount"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ImageScanJob represents a single image scanning task.
// These are created automatically by the SecurityScanConfig controller —
// users do not need to create them manually.
type ImageScanJob struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the image and scanner configuration for this job.
	// +required
	Spec ImageScanJobSpec `json:"spec"`

	// status reflects the observed state of the scan.
	// +optional
	Status ImageScanJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageScanJobList contains a list of ImageScanJob.
type ImageScanJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageScanJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageScanJob{}, &ImageScanJobList{})
}
