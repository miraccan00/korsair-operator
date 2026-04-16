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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterTargetPhase describes the connectivity state of a registered remote cluster.
type ClusterTargetPhase string

const (
	// ClusterTargetPhaseConnected means the operator can reach the remote cluster API.
	ClusterTargetPhaseConnected ClusterTargetPhase = "Connected"
	// ClusterTargetPhaseError means connectivity probe failed.
	ClusterTargetPhaseError ClusterTargetPhase = "Error"
)

// ClusterTargetSpec defines the desired state of ClusterTarget.
type ClusterTargetSpec struct {
	// displayName is a human-readable label for this cluster (e.g. "Production EU").
	// +required
	DisplayName string `json:"displayName"`

	// kubeconfigSecretRef references a Secret in the same namespace whose "kubeconfig" key
	// contains a valid kubeconfig with read access to the target cluster.
	// +required
	KubeconfigSecretRef corev1.LocalObjectReference `json:"kubeconfigSecretRef"`

	// excludeNamespaces is a list of namespaces to skip during image discovery in the
	// remote cluster. Uses the same semantics as SecurityScanConfig.
	// +optional
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`
}

// ClusterTargetStatus defines the observed state of ClusterTarget.
type ClusterTargetStatus struct {
	// phase summarises the connectivity state: Connected or Error.
	// +optional
	Phase ClusterTargetPhase `json:"phase,omitempty"`

	// lastProbeTime is when the connectivity check was last performed.
	// +optional
	LastProbeTime *metav1.Time `json:"lastProbeTime,omitempty"`

	// nodeCount is the number of nodes found in the remote cluster during the last probe.
	// +optional
	NodeCount int `json:"nodeCount,omitempty"`

	// message contains a human-readable description of the last probe result.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ct
// +kubebuilder:printcolumn:name="Display Name",type="string",JSONPath=".spec.displayName"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Nodes",type="integer",JSONPath=".status.nodeCount"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterTarget registers a remote Kubernetes cluster for image discovery.
// The operator will connect to the cluster using the referenced kubeconfig Secret
// and include its running images in the scan pipeline.
type ClusterTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterTargetSpec   `json:"spec,omitempty"`
	Status ClusterTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterTargetList contains a list of ClusterTarget.
type ClusterTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterTarget{}, &ClusterTargetList{})
}
