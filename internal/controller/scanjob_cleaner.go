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
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/miraccan00/korsair-operator/api/v1alpha1"
)

// ScanJobCleaner periodically deletes completed and failed ImageScanJob CRs
// that are older than Retention. The owned batch/v1 Jobs are cascade-deleted
// automatically via owner references.
//
// Register with the controller-runtime manager via mgr.Add(&ScanJobCleaner{...}).
type ScanJobCleaner struct {
	Client    client.Client
	Interval  time.Duration // how often to run (e.g. 5m)
	Retention time.Duration // delete jobs older than this (e.g. 1h)
}

// Start implements manager.Runnable. It blocks until ctx is cancelled.
func (c *ScanJobCleaner) Start(ctx context.Context) error {
	slog.Info("ScanJobCleaner started",
		"interval", c.Interval,
		"retention", c.Retention,
	)

	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.runCleanup(ctx)
		}
	}
}

func (c *ScanJobCleaner) runCleanup(ctx context.Context) {
	var jobList securityv1alpha1.ImageScanJobList
	if err := c.Client.List(ctx, &jobList); err != nil {
		slog.Error("ScanJobCleaner: failed to list ImageScanJobs", "err", err)
		return
	}

	cutoff := time.Now().Add(-c.Retention)
	deleted := 0

	for i := range jobList.Items {
		job := &jobList.Items[i]

		phase := job.Status.Phase
		if phase != securityv1alpha1.ImageScanJobPhaseCompleted &&
			phase != securityv1alpha1.ImageScanJobPhaseFailed {
			continue
		}

		if job.CreationTimestamp.After(cutoff) {
			continue
		}

		if err := c.Client.Delete(ctx, job); err != nil {
			if !apierrors.IsNotFound(err) {
				slog.Error("ScanJobCleaner: failed to delete ImageScanJob",
					"name", job.Name,
					"namespace", job.Namespace,
					"err", err,
				)
			}
			continue
		}

		slog.Info("ScanJobCleaner: deleted old ImageScanJob",
			"name", job.Name,
			"namespace", job.Namespace,
			"phase", phase,
			"age", time.Since(job.CreationTimestamp.Time).Round(time.Second),
		)
		deleted++
	}

	if deleted > 0 {
		slog.Info("ScanJobCleaner: cleanup cycle complete", "deleted", deleted)
	}
}
