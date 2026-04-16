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

// Package slack provides a minimal Slack Incoming Webhook client for BSO notifications.
package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client sends messages to a Slack Incoming Webhook URL.
type Client struct {
	WebhookURL string
	HTTPClient *http.Client
}

// NewClient returns a Client with a 10-second timeout.
func NewClient(webhookURL string) *Client {
	return &Client{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// slackPayload is the JSON body sent to the Incoming Webhook.
type slackPayload struct {
	Text   string  `json:"text,omitempty"`
	Blocks []block `json:"blocks,omitempty"`
}

type block struct {
	Type string   `json:"type"`
	Text *textObj `json:"text,omitempty"`
}

type textObj struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ScanSummary holds per-image result data for the batch notification message.
type ScanSummary struct {
	ConfigName   string
	Namespace    string
	ImageResults []ImageResult
}

// ImageResult holds the scan result for a single image.
type ImageResult struct {
	Image         string
	Scanner       string // "trivy" or "grype"
	Phase         string // Completed | Failed
	CriticalCount int
	HighCount     int
	MediumCount   int
	LowCount      int
	ReportCMName  string // ConfigMap name with CSV data
}

// SendScanReport sends a batch scan summary message to Slack.
func (c *Client) SendScanReport(summary ScanSummary) error {
	if c.WebhookURL == "" {
		return nil // webhook not configured — skip silently
	}

	text := formatScanReport(summary)
	payload := slackPayload{Text: text}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling slack payload: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("posting to slack webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// formatScanReport builds the Slack message text for a completed scan batch.
func formatScanReport(s ScanSummary) string {
	total := len(s.ImageResults)
	completed, failed := 0, 0
	for _, ir := range s.ImageResults {
		if ir.Phase == "Completed" {
			completed++
		} else {
			failed++
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, ":shield: *BSO Security Scan Report* | `%s`\n", s.ConfigName)
	fmt.Fprintf(&buf, "Total images: %d | :white_check_mark: Completed: %d | :x: Failed: %d\n\n", total, completed, failed)
	buf.WriteString("*Results per image:*\n")

	for _, ir := range s.ImageResults {
		scannerLabel := ""
		if ir.Scanner != "" {
			scannerLabel = fmt.Sprintf(" (%s)", ir.Scanner)
		}
		if ir.Phase == "Failed" {
			fmt.Fprintf(&buf, "• :x: `%s`%s — scan failed\n", ir.Image, scannerLabel)
			continue
		}
		severity := ":white_check_mark: Clean"
		if ir.CriticalCount > 0 {
			severity = fmt.Sprintf(":red_circle: Critical: %d | :large_orange_circle: High: %d | :large_yellow_circle: Medium: %d | :white_circle: Low: %d",
				ir.CriticalCount, ir.HighCount, ir.MediumCount, ir.LowCount)
		} else if ir.HighCount > 0 {
			severity = fmt.Sprintf(":large_orange_circle: High: %d | :large_yellow_circle: Medium: %d | :white_circle: Low: %d",
				ir.HighCount, ir.MediumCount, ir.LowCount)
		}
		fmt.Fprintf(&buf, "• `%s`%s\n  %s\n", ir.Image, scannerLabel, severity)
		if ir.ReportCMName != "" {
			fmt.Fprintf(&buf, "  :page_facing_up: `kubectl get cm %s -n %s -o jsonpath='{.data.report\\.csv}'`\n", ir.ReportCMName, s.Namespace)
		}
	}

	return buf.String()
}
