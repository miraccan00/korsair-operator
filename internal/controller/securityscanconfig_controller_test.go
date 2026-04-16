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

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
	bsoslack "github.com/miraccan00/korsair-operator/internal/slack"
)

var _ = Describe("computeRequeue", func() {
	r := &SecurityScanConfigReconciler{DiscoveryInterval: 5 * time.Minute}

	It("returns DiscoveryInterval when schedule is empty", func() {
		config := &securityv1alpha1.SecurityScanConfig{}
		Expect(r.computeRequeue(config)).To(Equal(5 * time.Minute))
	})

	It("returns a positive duration for a valid standard cron expression", func() {
		config := &securityv1alpha1.SecurityScanConfig{
			Spec: securityv1alpha1.SecurityScanConfigSpec{
				Schedule: "0 2 * * *", // daily at 02:00
			},
		}
		d := r.computeRequeue(config)
		Expect(d).To(BeNumerically(">", 0))
		Expect(d).To(BeNumerically("<=", 24*time.Hour))
	})

	It("falls back to DiscoveryInterval for an invalid cron expression", func() {
		config := &securityv1alpha1.SecurityScanConfig{
			Spec: securityv1alpha1.SecurityScanConfigSpec{
				Schedule: "not-a-cron-expression",
			},
		}
		Expect(r.computeRequeue(config)).To(Equal(5 * time.Minute))
	})
})

var _ = Describe("exceedsThresholds", func() {
	It("returns true when thresholds is nil (no gate)", func() {
		results := []bsoslack.ImageResult{{CriticalCount: 0, HighCount: 0}}
		Expect(exceedsThresholds(results, nil)).To(BeTrue())
	})

	It("returns true when criticalCount exceeds the critical threshold", func() {
		thresholds := &securityv1alpha1.ScanThresholds{Critical: 0, High: 5}
		results := []bsoslack.ImageResult{{CriticalCount: 1, HighCount: 0}}
		Expect(exceedsThresholds(results, thresholds)).To(BeTrue())
	})

	It("returns true when highCount exceeds the high threshold", func() {
		thresholds := &securityv1alpha1.ScanThresholds{Critical: 0, High: 5}
		results := []bsoslack.ImageResult{{CriticalCount: 0, HighCount: 6}}
		Expect(exceedsThresholds(results, thresholds)).To(BeTrue())
	})

	It("returns false when all images are within thresholds", func() {
		thresholds := &securityv1alpha1.ScanThresholds{Critical: 2, High: 10}
		results := []bsoslack.ImageResult{
			{CriticalCount: 2, HighCount: 10}, // exactly at threshold — not exceeded
		}
		Expect(exceedsThresholds(results, thresholds)).To(BeFalse())
	})

	It("returns true when at least one image exceeds thresholds", func() {
		thresholds := &securityv1alpha1.ScanThresholds{Critical: 0, High: 5}
		results := []bsoslack.ImageResult{
			{CriticalCount: 0, HighCount: 3},
			{CriticalCount: 1, HighCount: 2}, // this one exceeds critical threshold
		}
		Expect(exceedsThresholds(results, thresholds)).To(BeTrue())
	})
})

var _ = Describe("imageToJobName", func() {
	It("is deterministic for the same image and scanner (no digest)", func() {
		name1 := imageToJobName("docker.io/library/nginx:1.25", "trivy", "")
		name2 := imageToJobName("docker.io/library/nginx:1.25", "trivy", "")
		Expect(name1).To(Equal(name2))
	})

	It("differs by scanner suffix (no digest)", func() {
		trivyName := imageToJobName("docker.io/library/nginx:1.25", "trivy", "")
		grypeName := imageToJobName("docker.io/library/nginx:1.25", "grype", "")
		Expect(trivyName).NotTo(Equal(grypeName))
		Expect(trivyName).To(HaveSuffix("-trivy"))
		Expect(grypeName).To(HaveSuffix("-grype"))
	})

	It("uses digest prefix when digest is available", func() {
		digest := "sha256:09fb0c6289cefaad8c74c7e5fd6758ad6906ab8f57f1350d9f4eb5a7df45ff8b"
		name := imageToJobName("docker.io/library/nginx:1.25", "trivy", digest)
		Expect(name).To(Equal("scan-09fb0c62-trivy"))
	})

	It("deduplicates two tags pointing to the same digest", func() {
		digest := "sha256:09fb0c6289cefaad8c74c7e5fd6758ad6906ab8f57f1350d9f4eb5a7df45ff8b"
		name1 := imageToJobName("nginx:1.25", "trivy", digest)
		name2 := imageToJobName("nginx:latest", "trivy", digest)
		Expect(name1).To(Equal(name2))
	})
})
