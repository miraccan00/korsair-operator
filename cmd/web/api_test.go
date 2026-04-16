/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// ── handlePutJobReport ────────────────────────────────────────────────────────

func TestHandlePutJobReport_ValidRequest_WritesToDB(t *testing.T) {
	// Arrange
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	const token = "test-secret"
	body := reportWriteRequest{
		Scanner: "trivy",
		Rows: []reportRow{
			{
				Image: "nginx:latest", Target: "debian", Library: "libssl3",
				VulnerabilityID: "CVE-2024-0001", Severity: "CRITICAL",
				Status: "fixed", InstalledVersion: "3.0.1", FixedVersion: "3.0.2",
				Title: "Buffer overflow",
			},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO scan_reports`).
		WithArgs("default", "scan-abc", "trivy").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM scan_report_rows`).
		WithArgs("default", "scan-abc").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectPrepare(`INSERT INTO scan_report_rows`)
	mock.ExpectExec(`INSERT INTO scan_report_rows`).
		WithArgs(
			"default", "scan-abc",
			"nginx:latest", "debian", "libssl3",
			"CVE-2024-0001", "CRITICAL", "fixed", "3.0.1", "3.0.2", "Buffer overflow",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	bodyJSON, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/jobs/default/scan-abc/report",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Korsair-Token", token)
	req.SetPathValue("namespace", "default")
	req.SetPathValue("name", "scan-abc")
	w := httptest.NewRecorder()

	// Act
	handlePutJobReport(db, token)(w, req)

	// Assert
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d — body: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet DB expectations: %v", err)
	}
}

func TestHandlePutJobReport_WrongToken_Returns401(t *testing.T) {
	// Arrange
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/jobs/default/scan-abc/report",
		strings.NewReader(`{}`))
	req.Header.Set("X-Korsair-Token", "wrong-token")
	req.SetPathValue("namespace", "default")
	req.SetPathValue("name", "scan-abc")
	w := httptest.NewRecorder()

	// Act
	handlePutJobReport(db, "correct-token")(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandlePutJobReport_MalformedBody_Returns400(t *testing.T) {
	// Arrange
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/jobs/default/scan-abc/report",
		strings.NewReader(`not-valid-json`))
	req.SetPathValue("namespace", "default")
	req.SetPathValue("name", "scan-abc")
	w := httptest.NewRecorder()

	// Act
	handlePutJobReport(db, "")(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── handleJobReport ───────────────────────────────────────────────────────────

func TestHandleJobReport_RowsFound_ReturnsCSVAndJSON(t *testing.T) {
	// Arrange
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	rows := sqlmock.NewRows([]string{
		"image", "target", "library", "vulnerability_id",
		"severity", "status", "installed_version", "fixed_version", "title",
	}).AddRow(
		"nginx:latest", "debian", "libssl3", "CVE-2024-0001",
		"CRITICAL", "fixed", "3.0.1", "3.0.2", "Buffer overflow",
	)
	mock.ExpectQuery(`SELECT image`).
		WithArgs("default", "scan-abc").
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/default/scan-abc/report", nil)
	req.SetPathValue("namespace", "default")
	req.SetPathValue("name", "scan-abc")
	w := httptest.NewRecorder()

	// Act
	handleJobReport(db)(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp reportResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// header row + 1 data row
	if len(resp.Rows) != 2 {
		t.Errorf("expected 2 rows (header + 1 data), got %d", len(resp.Rows))
	}
	if !strings.Contains(resp.RawCSV, "CVE-2024-0001") {
		t.Errorf("CSV should contain CVE-2024-0001, got: %s", resp.RawCSV)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet DB expectations: %v", err)
	}
}

func TestHandleJobReport_NoRows_Returns404(t *testing.T) {
	// Arrange
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`SELECT image`).
		WithArgs("default", "nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{
			"image", "target", "library", "vulnerability_id",
			"severity", "status", "installed_version", "fixed_version", "title",
		}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/default/nonexistent/report", nil)
	req.SetPathValue("namespace", "default")
	req.SetPathValue("name", "nonexistent")
	w := httptest.NewRecorder()

	// Act
	handleJobReport(db)(w, req)

	// Assert
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d — body: %s", w.Code, w.Body.String())
	}
}

// ── defaultStr ───────────────────────────────────────────────────────────────

func TestDefaultStr(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback string
		want     string
	}{
		// Arrange + Act + Assert in table form
		{"non-empty value returned as-is", "5432", "5433", "5432"},
		{"empty value falls back to default", "", "5432", "5432"},
		{"both empty returns empty default", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultStr(tt.value, tt.fallback)
			if got != tt.want {
				t.Errorf("defaultStr(%q, %q) = %q, want %q", tt.value, tt.fallback, got, tt.want)
			}
		})
	}
}
