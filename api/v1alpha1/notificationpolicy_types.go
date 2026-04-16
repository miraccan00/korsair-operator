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

// NotificationPolicySpec defines the desired notification routing and threshold configuration.
type NotificationPolicySpec struct {
	// webhookURL is the Slack Incoming Webhook URL to send notifications to.
	// Must be a non-empty HTTPS URL.
	// +required
	WebhookURL string `json:"webhookURL"`

	// thresholds defines optional CVE severity limits that gate notifications.
	// When set, a notification is only sent if at least one image exceeds a threshold.
	// When omitted, all scan completions trigger a notification.
	// +optional
	Thresholds *ScanThresholds `json:"thresholds,omitempty"`

	// severityFilter limits which severity levels are included in the notification.
	// Accepted values: CRITICAL, HIGH, MEDIUM, LOW.
	// When omitted, all severities are included.
	// +optional
	SeverityFilter []string `json:"severityFilter,omitempty"`
}

// NotificationPolicyStatus defines the observed state of NotificationPolicy.
type NotificationPolicyStatus struct {
	// conditions represent the current state of the NotificationPolicy.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=np
// +kubebuilder:printcolumn:name="Webhook",type="string",JSONPath=".spec.webhookURL"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NotificationPolicy defines per-team Slack webhook routing and CVE severity thresholds.
// A SecurityScanConfig can reference a NotificationPolicy to override the global webhook URL
// and apply threshold-based notification gating.
type NotificationPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the notification routing and threshold configuration.
	// +required
	Spec NotificationPolicySpec `json:"spec"`

	// status reflects the observed state of the policy.
	// +optional
	Status NotificationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NotificationPolicyList contains a list of NotificationPolicy.
type NotificationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotificationPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NotificationPolicy{}, &NotificationPolicyList{})
}
