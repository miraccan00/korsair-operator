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
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
)

// NotificationPolicyReconciler reconciles a NotificationPolicy object.
type NotificationPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=notificationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=notificationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=notificationpolicies/finalizers,verbs=update

// Reconcile validates the NotificationPolicy and sets a Ready status condition.
func (r *NotificationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	np := &securityv1alpha1.NotificationPolicy{}
	if err := r.Get(ctx, req.NamespacedName, np); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling NotificationPolicy", "name", np.Name, "namespace", np.Namespace)

	condStatus, condReason, condMsg := validateWebhookURL(np.Spec.WebhookURL)

	apimeta.SetStatusCondition(&np.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             condStatus,
		Reason:             condReason,
		ObservedGeneration: np.Generation,
		Message:            condMsg,
	})

	if err := r.Status().Update(ctx, np); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// validateWebhookURL returns the condition status, reason and message for the given webhook URL.
func validateWebhookURL(url string) (metav1.ConditionStatus, string, string) {
	if url == "" {
		return metav1.ConditionFalse, "InvalidWebhookURL", "webhookURL must not be empty"
	}
	if !strings.HasPrefix(url, "https://") {
		return metav1.ConditionFalse, "InvalidWebhookURL", "webhookURL must be an HTTPS URL"
	}
	return metav1.ConditionTrue, "ValidWebhook", "NotificationPolicy is configured and ready"
}

// SetupWithManager registers the controller with the Manager.
func (r *NotificationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.NotificationPolicy{}).
		Named("notificationpolicy").
		Complete(r)
}
