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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
)

var _ = Describe("NotificationPolicy Controller", func() {
	const (
		npTimeout  = 10 * time.Second
		npInterval = 250 * time.Millisecond
	)

	Context("validateWebhookURL", func() {
		It("returns True for a valid HTTPS URL", func() {
			status, reason, _ := validateWebhookURL("https://hooks.slack.com/services/T/B/X")
			Expect(status).To(Equal(metav1.ConditionTrue))
			Expect(reason).To(Equal("ValidWebhook"))
		})

		It("returns False for an empty URL", func() {
			status, reason, _ := validateWebhookURL("")
			Expect(status).To(Equal(metav1.ConditionFalse))
			Expect(reason).To(Equal("InvalidWebhookURL"))
		})

		It("returns False for an HTTP URL", func() {
			status, reason, _ := validateWebhookURL("http://hooks.slack.com/services/T/B/X")
			Expect(status).To(Equal(metav1.ConditionFalse))
			Expect(reason).To(Equal("InvalidWebhookURL"))
		})
	})

	Context("when reconciling a NotificationPolicy with a valid webhookURL", func() {
		It("should set Ready=True condition", func() {
			np := &securityv1alpha1.NotificationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "np-valid-test",
					Namespace: "default",
				},
				Spec: securityv1alpha1.NotificationPolicySpec{
					WebhookURL: "https://hooks.slack.com/services/T/B/X",
				},
			}
			Expect(k8sClient.Create(ctx, np)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, np)
			})

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: "np-valid-test", Namespace: "default",
				}, np)).To(Succeed())
				cond := apimeta.FindStatusCondition(np.Status.Conditions, "Ready")
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			}, npTimeout, npInterval).Should(Succeed())
		})
	})

	Context("when reconciling a NotificationPolicy with an empty webhookURL", func() {
		It("should set Ready=False condition", func() {
			np := &securityv1alpha1.NotificationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "np-invalid-test",
					Namespace: "default",
				},
				Spec: securityv1alpha1.NotificationPolicySpec{
					WebhookURL: "",
				},
			}
			Expect(k8sClient.Create(ctx, np)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, np)
			})

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: "np-invalid-test", Namespace: "default",
				}, np)).To(Succeed())
				cond := apimeta.FindStatusCondition(np.Status.Conditions, "Ready")
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			}, npTimeout, npInterval).Should(Succeed())
		})
	})
})
