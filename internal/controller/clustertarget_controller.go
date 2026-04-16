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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
)

// ClusterTargetReconciler reconciles ClusterTarget objects.
type ClusterTargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=clustertargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=clustertargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=security.blacksyrius.com,resources=clustertargets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile probes the remote cluster referenced by the ClusterTarget and updates status.
func (r *ClusterTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	ct := &securityv1alpha1.ClusterTarget{}
	if err := r.Get(ctx, req.NamespacedName, ct); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Probing remote cluster", "clusterTarget", ct.Name, "displayName", ct.Spec.DisplayName)

	phase, nodeCount, msg := r.probeCluster(ctx, ct)

	now := metav1.Now()
	base := ct.DeepCopy()
	ct.Status.Phase = phase
	ct.Status.LastProbeTime = &now
	ct.Status.NodeCount = nodeCount
	ct.Status.Message = msg

	if err := r.Status().Patch(ctx, ct, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, err
	}

	// Re-probe every 5 minutes.
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// probeCluster reads the kubeconfig secret, builds a client, and checks connectivity.
func (r *ClusterTargetReconciler) probeCluster(
	ctx context.Context,
	ct *securityv1alpha1.ClusterTarget,
) (phase securityv1alpha1.ClusterTargetPhase, nodeCount int, message string) {
	// Fetch the kubeconfig Secret.
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: ct.Namespace,
		Name:      ct.Spec.KubeconfigSecretRef.Name,
	}, secret)
	if err != nil {
		return securityv1alpha1.ClusterTargetPhaseError, 0,
			fmt.Sprintf("failed to get kubeconfig secret %q: %v", ct.Spec.KubeconfigSecretRef.Name, err)
	}

	kubeconfigBytes, ok := secret.Data["kubeconfig"]
	if !ok {
		return securityv1alpha1.ClusterTargetPhaseError, 0,
			fmt.Sprintf("secret %q has no 'kubeconfig' key", ct.Spec.KubeconfigSecretRef.Name)
	}

	// Build a REST config from the kubeconfig bytes.
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		return securityv1alpha1.ClusterTargetPhaseError, 0,
			fmt.Sprintf("invalid kubeconfig: %v", err)
	}

	// Build a controller-runtime client.
	remoteClient, err := client.New(restCfg, client.Options{Scheme: r.Scheme})
	if err != nil {
		return securityv1alpha1.ClusterTargetPhaseError, 0,
			fmt.Sprintf("failed to build remote client: %v", err)
	}

	// List nodes to verify connectivity and count cluster size.
	nodeList := &corev1.NodeList{}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := remoteClient.List(probeCtx, nodeList); err != nil {
		return securityv1alpha1.ClusterTargetPhaseError, 0,
			fmt.Sprintf("connectivity probe failed: %v", err)
	}

	n := len(nodeList.Items)
	return securityv1alpha1.ClusterTargetPhaseConnected, n,
		fmt.Sprintf("connected to remote cluster; %d node(s) found", n)
}

// BuildClientFromKubeconfigSecret builds a controller-runtime client from a kubeconfig Secret.
// The Secret must contain a "kubeconfig" key. Used by SecurityScanConfigReconciler for
// fan-out image discovery across registered ClusterTargets.
func BuildClientFromKubeconfigSecret(secret *corev1.Secret, scheme *runtime.Scheme) (client.Client, error) {
	kubeconfigBytes, ok := secret.Data["kubeconfig"]
	if !ok {
		return nil, fmt.Errorf("secret %q/%q has no 'kubeconfig' key", secret.Namespace, secret.Name)
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid kubeconfig in secret %q/%q: %w", secret.Namespace, secret.Name, err)
	}
	return client.New(restCfg, client.Options{Scheme: scheme})
}

// SetupWithManager registers the ClusterTarget controller with the Manager.
func (r *ClusterTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.ClusterTarget{}).
		Named("clustertarget").
		Complete(r)
}
