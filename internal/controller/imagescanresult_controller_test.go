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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
)

const (
	nexusRegistry = "nexus.company.com"
	nexusImage    = "nexus.company.com/app/api:v1"
)

// ── rewriteRegistry ───────────────────────────────────────────────────────────

var _ = Describe("rewriteRegistry", func() {
	Context("when source and target are set and the image has a matching registry prefix", func() {
		It("replaces the registry prefix with the target", func() {
			// Arrange
			image := nexusImage
			source := nexusRegistry
			target := "registry.company.com"

			// Act
			result := rewriteRegistry(image, source, target)

			// Assert
			Expect(result).To(Equal("registry.company.com/app/api:v1"))
		})
	})

	Context("when the image registry does not match source", func() {
		It("returns the image unchanged", func() {
			// Arrange
			image := "docker.io/library/nginx:latest"
			source := nexusRegistry
			target := "registry.company.com"

			// Act
			result := rewriteRegistry(image, source, target)

			// Assert
			Expect(result).To(Equal("docker.io/library/nginx:latest"))
		})
	})

	Context("when source is empty", func() {
		It("returns the image unchanged", func() {
			// Arrange
			image := nexusImage

			// Act
			result := rewriteRegistry(image, "", "registry.company.com")

			// Assert
			Expect(result).To(Equal(nexusImage))
		})
	})

	Context("when the image has no registry prefix (short name like nginx:latest)", func() {
		It("returns the image unchanged", func() {
			// Arrange
			image := "nginx:latest"

			// Act
			result := rewriteRegistry(image, nexusRegistry, "registry.company.com")

			// Assert
			Expect(result).To(Equal("nginx:latest"))
		})
	})

	Context("when source and target are equal", func() {
		It("returns the image unchanged without allocating a new string", func() {
			// Arrange
			image := nexusImage
			same := nexusRegistry

			// Act
			result := rewriteRegistry(image, same, same)

			// Assert
			Expect(result).To(Equal(image))
		})
	})
})

// ── vulnToRow ────────────────────────────────────────────────────────────────

var _ = Describe("vulnToRow", func() {
	It("converts all vulnRecord fields to the matching reportRow wire fields", func() {
		// Arrange
		rec := vulnRecord{
			Image:            "nginx:latest",
			Target:           "nginx:latest (debian 12)",
			Library:          "libssl3",
			VulnerabilityID:  "CVE-2024-0001",
			Severity:         "CRITICAL",
			Status:           "fixed",
			InstalledVersion: "3.0.1",
			FixedVersion:     "3.0.2",
			Title:            "Buffer overflow in libssl",
		}

		// Act
		row := vulnToRow(rec)

		// Assert
		Expect(row.Image).To(Equal(rec.Image))
		Expect(row.Target).To(Equal(rec.Target))
		Expect(row.Library).To(Equal(rec.Library))
		Expect(row.VulnerabilityID).To(Equal(rec.VulnerabilityID))
		Expect(row.Severity).To(Equal(rec.Severity))
		Expect(row.Status).To(Equal(rec.Status))
		Expect(row.InstalledVersion).To(Equal(rec.InstalledVersion))
		Expect(row.FixedVersion).To(Equal(rec.FixedVersion))
		Expect(row.Title).To(Equal(rec.Title))
	})

	It("preserves empty optional fields without panicking", func() {
		// Arrange
		rec := vulnRecord{
			Image:           "scratch:latest",
			VulnerabilityID: "CVE-2024-0002",
			Severity:        "LOW",
		}

		// Act
		row := vulnToRow(rec)

		// Assert
		Expect(row.FixedVersion).To(BeEmpty())
		Expect(row.Title).To(BeEmpty())
	})
})

// ── scannerJobSuffix ──────────────────────────────────────────────────────────

var _ = Describe("scannerJobSuffix", func() {
	DescribeTable("returns the correct suffix for each scanner",
		func(scanner, expected string) {
			// Act + Assert (single-value functions don't benefit from splitting Arrange)
			Expect(scannerJobSuffix(scanner)).To(Equal(expected))
		},
		Entry("trivy → -trivy", "trivy", "-trivy"),
		Entry("grype → -grype", "grype", "-grype"),
		Entry("unknown → defaults to -trivy", "unknown", "-trivy"),
		Entry("empty string → defaults to -trivy", "", "-trivy"),
	)
})

// ── parseGrypeJSON ────────────────────────────────────────────────────────────

var _ = Describe("parseGrypeJSON", func() {
	Context("with a valid Grype output containing mixed severities", func() {
		It("returns correct counts and fully-populated records", func() {
			// Arrange
			out := grypeOutput{
				Matches: []grypeMatch{
					{
						Vulnerability: grypeVuln{
							ID:       "CVE-2021-001",
							Severity: "Critical",
							Fix:      grypeFix{State: "fixed", Versions: []string{"1.2.0"}},
						},
						Artifact: grypeArtifact{Name: "libssl", Version: "1.0.0"},
					},
					{
						Vulnerability: grypeVuln{ID: "CVE-2021-002", Severity: "High"},
						Artifact:      grypeArtifact{Name: "libz", Version: "1.2.0"},
					},
					{
						Vulnerability: grypeVuln{ID: "CVE-2021-003", Severity: "Medium"},
						Artifact:      grypeArtifact{Name: "libpng", Version: "1.6.0"},
					},
					{
						Vulnerability: grypeVuln{ID: "CVE-2021-004", Severity: "Low"},
						Artifact:      grypeArtifact{Name: "libc", Version: "2.17"},
					},
				},
			}
			data, err := json.Marshal(out)
			Expect(err).NotTo(HaveOccurred())

			// Act
			counts, records, parseErr := parseGrypeJSON("nginx:latest", data)

			// Assert
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(counts["CRITICAL"]).To(Equal(1))
			Expect(counts["HIGH"]).To(Equal(1))
			Expect(counts["MEDIUM"]).To(Equal(1))
			Expect(counts["LOW"]).To(Equal(1))
			Expect(records).To(HaveLen(4))
			Expect(records[0].VulnerabilityID).To(Equal("CVE-2021-001"))
			Expect(records[0].FixedVersion).To(Equal("1.2.0"))
			Expect(records[0].Status).To(Equal("fixed"))
		})
	})

	Context("with an empty matches list", func() {
		It("returns zero counts and an empty record slice without error", func() {
			// Arrange
			data, _ := json.Marshal(grypeOutput{Matches: []grypeMatch{}})

			// Act
			counts, records, err := parseGrypeJSON("nginx:latest", data)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(counts["CRITICAL"]).To(Equal(0))
			Expect(records).To(BeEmpty())
		})
	})

	Context("with input that contains no JSON object", func() {
		It("returns a descriptive error", func() {
			// Arrange
			input := []byte("not json at all")

			// Act
			_, _, err := parseGrypeJSON("nginx:latest", input)

			// Assert
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no Grype output found"))
		})
	})

	// Regression: in non-TTY Kubernetes pods Grype writes JSON structured log
	// entries to stderr which Kubernetes merges with stdout. The parser must
	// locate the actual scan result by finding the "matches" key, not just the
	// first '{'. Without this logic it would parse a log entry (e.g.
	// {"level":"info",...}) and silently return 0 vulnerabilities.
	Context("when JSON structured log lines appear before the scan result (Kubernetes pod log mixing)", func() {
		It("skips log noise and parses the actual scan output", func() {
			// Arrange
			logNoise := `{"level":"info","time":"2026-01-01T00:00:00Z","msg":"Loading DB"}` + "\n" +
				`{"level":"info","time":"2026-01-01T00:00:01Z","msg":"Scanning","image":"nginx:latest"}` + "\n"
			scan := grypeOutput{
				Matches: []grypeMatch{
					{
						Vulnerability: grypeVuln{ID: "CVE-2024-001", Severity: "Critical"},
						Artifact:      grypeArtifact{Name: "libssl", Version: "1.0.0"},
					},
					{
						Vulnerability: grypeVuln{ID: "CVE-2024-002", Severity: "High"},
						Artifact:      grypeArtifact{Name: "libz", Version: "1.2.0"},
					},
				},
			}
			scanJSON, _ := json.Marshal(scan)
			mixed := []byte(logNoise + string(scanJSON))

			// Act
			counts, records, err := parseGrypeJSON("nginx:latest", mixed)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(counts["CRITICAL"]).To(Equal(1), "must count from scan output, not log entry")
			Expect(counts["HIGH"]).To(Equal(1))
			Expect(records).To(HaveLen(2))
		})
	})
})

// ── parseTrivyJSON ────────────────────────────────────────────────────────────

var _ = Describe("parseTrivyJSON", func() {
	Context("with a valid Trivy output containing multiple targets", func() {
		It("aggregates counts across all targets and returns one record per CVE", func() {
			// Arrange
			out := trivyOutput{
				Results: []trivyResult{
					{
						Target: "nginx:latest (debian 12.4)",
						Vulnerabilities: []trivyVuln{
							{VulnerabilityID: "CVE-2023-001", Severity: "CRITICAL", PkgName: "libssl",
								InstalledVersion: "3.0.1", FixedVersion: "3.0.2", Status: "fixed"},
							{VulnerabilityID: "CVE-2023-002", Severity: "HIGH", PkgName: "libz"},
						},
					},
					{
						Target: "usr/local/bin/app (gobinary)",
						Vulnerabilities: []trivyVuln{
							{VulnerabilityID: "CVE-2023-003", Severity: "MEDIUM", PkgName: "golang.org/x/net"},
						},
					},
				},
			}
			data, _ := json.Marshal(out)

			// Act
			counts, records, err := parseTrivyJSON("nginx:latest", data)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(counts["CRITICAL"]).To(Equal(1))
			Expect(counts["HIGH"]).To(Equal(1))
			Expect(counts["MEDIUM"]).To(Equal(1))
			Expect(counts["LOW"]).To(Equal(0))
			Expect(records).To(HaveLen(3))
			Expect(records[0].Target).To(Equal("nginx:latest (debian 12.4)"))
			Expect(records[0].FixedVersion).To(Equal("3.0.2"))
		})
	})

	Context("when Trivy output has no vulnerabilities in any target", func() {
		It("returns zero counts without error", func() {
			// Arrange
			out := trivyOutput{
				Results: []trivyResult{
					{Target: "scratch:latest", Vulnerabilities: nil},
				},
			}
			data, _ := json.Marshal(out)

			// Act
			counts, records, err := parseTrivyJSON("scratch:latest", data)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(counts["CRITICAL"]).To(Equal(0))
			Expect(records).To(BeEmpty())
		})
	})

	Context("when the input is not valid JSON", func() {
		It("returns an error", func() {
			// Arrange
			input := []byte("plain text — no JSON here")

			// Act
			_, _, err := parseTrivyJSON("nginx:latest", input)

			// Assert
			Expect(err).To(HaveOccurred())
		})
	})
})

// ── postReportToAPI ───────────────────────────────────────────────────────────

var _ = Describe("postReportToAPI", func() {
	var (
		reconciler *ImageScanJobReconciler
		isj        *securityv1alpha1.ImageScanJob
		records    []vulnRecord
	)

	BeforeEach(func() {
		reconciler = &ImageScanJobReconciler{
			Client:     k8sClient,
			Scheme:     k8sClient.Scheme(),
			TrivyImage: "aquasec/trivy:0.58.1",
			GrypeImage: "anchore/grype:v0.90.0",
		}
		isj = &securityv1alpha1.ImageScanJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scan-report-test",
				Namespace: "default",
			},
			Spec: securityv1alpha1.ImageScanJobSpec{
				Image:   "nginx:latest",
				Scanner: "trivy",
			},
		}
		records = []vulnRecord{
			{
				Image: "nginx:latest", Target: "debian", Library: "libssl3",
				VulnerabilityID: "CVE-2024-0001", Severity: "CRITICAL",
				Status: "fixed", InstalledVersion: "3.0.1", FixedVersion: "3.0.2",
				Title: "Buffer overflow",
			},
		}
	})

	Context("when the API server accepts the report", func() {
		It("sends a PUT request with the correct token and payload and returns nil", func() {
			// Arrange
			var receivedToken, receivedMethod, receivedPath string
			var receivedBody reportWriteRequest

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				receivedPath = r.URL.Path
				receivedToken = r.Header.Get("X-Korsair-Token")
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			GinkgoT().Setenv("KORSAIR_API_URL", server.URL)
			GinkgoT().Setenv("KORSAIR_INTERNAL_TOKEN", "test-token")

			// Act
			err := reconciler.postReportToAPI(context.Background(), isj, records)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedMethod).To(Equal("PUT"))
			Expect(receivedPath).To(Equal("/api/v1/jobs/default/scan-report-test/report"))
			Expect(receivedToken).To(Equal("test-token"))
			Expect(receivedBody.Scanner).To(Equal("trivy"))
			Expect(receivedBody.Rows).To(HaveLen(1))
			Expect(receivedBody.Rows[0].VulnerabilityID).To(Equal("CVE-2024-0001"))
		})
	})

	Context("when the API server returns a 5xx error on every attempt", func() {
		It("returns an error after exhausting retries", func() {
			// Arrange
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, "db unavailable")
			}))
			defer server.Close()

			GinkgoT().Setenv("KORSAIR_API_URL", server.URL)
			GinkgoT().Setenv("KORSAIR_INTERNAL_TOKEN", "")

			// Override the HTTP client to reduce backoff delay in tests.
			original := reportHTTPClient
			reportHTTPClient = &http.Client{Timeout: 1 * time.Second}
			DeferCleanup(func() { reportHTTPClient = original })

			// Act
			err := reconciler.postReportToAPI(context.Background(), isj, records)

			// Assert
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("post report after retries"))
			Expect(callCount).To(Equal(3))
		})
	})

	Context("when KORSAIR_API_URL is not set", func() {
		It("returns an error immediately without making any HTTP call", func() {
			// Arrange
			GinkgoT().Setenv("KORSAIR_API_URL", "")

			// Act
			err := reconciler.postReportToAPI(context.Background(), isj, records)

			// Assert
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("KORSAIR_API_URL not set"))
		})
	})
})

// ── ImageScanJob controller — Job creation ───────────────────────────────────

var _ = Describe("ImageScanJob controller — Job creation", func() {
	const (
		isjTimeout  = 10 * time.Second
		isjInterval = 250 * time.Millisecond
	)

	Context("when scanner=grype", func() {
		It("creates a Kubernetes Job with -grype suffix and --quiet flag for JSON log suppression", func() {
			// Arrange
			isj := &securityv1alpha1.ImageScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scan-test-grype-01",
					Namespace: "default",
				},
				Spec: securityv1alpha1.ImageScanJobSpec{
					Image:     "nginx:latest",
					Scanner:   "grype",
					Source:    "kubernetes",
					ConfigRef: "test-config",
				},
			}
			Expect(k8sClient.Create(ctx, isj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, isj) })

			// Act + Assert
			Eventually(func(g Gomega) {
				job := &batchv1.Job{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "scan-test-grype-01" + grypeJobSuffix,
					Namespace: "default",
				}, job)).To(Succeed())
				g.Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
				g.Expect(job.Spec.Template.Spec.Containers[0].Name).To(Equal("grype"))
				g.Expect(job.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("grype"))
				// --quiet suppresses structured JSON logs emitted to stderr in non-TTY pods
				g.Expect(job.Spec.Template.Spec.Containers[0].Args).To(ContainElement("--quiet"))
			}, isjTimeout, isjInterval).Should(Succeed())
		})
	})

	Context("when scanner=trivy", func() {
		It("creates a Kubernetes Job with -trivy suffix and image scan arguments", func() {
			// Arrange
			isj := &securityv1alpha1.ImageScanJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scan-test-trivy-01",
					Namespace: "default",
				},
				Spec: securityv1alpha1.ImageScanJobSpec{
					Image:     "nginx:latest",
					Scanner:   "trivy",
					Source:    "kubernetes",
					ConfigRef: "test-config",
				},
			}
			Expect(k8sClient.Create(ctx, isj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, isj) })

			// Act + Assert
			Eventually(func(g Gomega) {
				job := &batchv1.Job{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "scan-test-trivy-01" + trivyJobSuffix,
					Namespace: "default",
				}, job)).To(Succeed())
				g.Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
				g.Expect(job.Spec.Template.Spec.Containers[0].Name).To(Equal("trivy"))
				g.Expect(job.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("trivy"))
				g.Expect(job.Spec.Template.Spec.Containers[0].Args).To(ContainElements("image", "--format", "json"))
			}, isjTimeout, isjInterval).Should(Succeed())
		})
	})
})
