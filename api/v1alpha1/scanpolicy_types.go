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

// ScanPolicySpec defines CVE threshold values and action rules.
type ScanPolicySpec struct {
	// thresholds defines the CVE count limits that trigger actions.
	// +optional
	Thresholds ScanThresholds `json:"thresholds,omitempty"`
}

// ScanThresholds defines severity count limits.
type ScanThresholds struct {
	// critical is the maximum number of CRITICAL CVEs allowed before alerting.
	// +optional
	// +kubebuilder:default=0
	Critical int `json:"critical,omitempty"`

	// high is the maximum number of HIGH CVEs allowed before alerting.
	// +optional
	// +kubebuilder:default=5
	High int `json:"high,omitempty"`
}

// ScanPolicyStatus defines the observed state of ScanPolicy.
type ScanPolicyStatus struct {
	// conditions represent the current state of the ScanPolicy.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sp

// ScanPolicy defines CVE threshold values and alert rules applied to scan results.
// (v0.1: CRD is defined but enforcement is not yet implemented.)
type ScanPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the policy thresholds.
	// +required
	Spec ScanPolicySpec `json:"spec"`

	// status reflects the observed state of the policy.
	// +optional
	Status ScanPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScanPolicyList contains a list of ScanPolicy.
type ScanPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScanPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScanPolicy{}, &ScanPolicyList{})
}
