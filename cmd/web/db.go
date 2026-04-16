/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// reportRow is the JSON shape exchanged with the operator and persisted per CVE.
type reportRow struct {
	Image            string `json:"image"`
	Target           string `json:"target"`
	Library          string `json:"library"`
	VulnerabilityID  string `json:"vulnerabilityId"`
	Severity         string `json:"severity"`
	Status           string `json:"status"`
	InstalledVersion string `json:"installedVersion"`
	FixedVersion     string `json:"fixedVersion"`
	Title            string `json:"title"`
}

// reportWriteRequest is the PUT /jobs/{ns}/{name}/report body.
type reportWriteRequest struct {
	Scanner string      `json:"scanner"`
	Rows    []reportRow `json:"rows"`
}

const schemaDDL = `
CREATE TABLE IF NOT EXISTS scan_reports (
    isj_namespace TEXT NOT NULL,
    isj_name      TEXT NOT NULL,
    scanner       TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (isj_namespace, isj_name)
);

CREATE TABLE IF NOT EXISTS scan_report_rows (
    id                BIGSERIAL PRIMARY KEY,
    isj_namespace     TEXT NOT NULL,
    isj_name          TEXT NOT NULL,
    image             TEXT NOT NULL DEFAULT '',
    target            TEXT NOT NULL DEFAULT '',
    library           TEXT NOT NULL DEFAULT '',
    vulnerability_id  TEXT NOT NULL DEFAULT '',
    severity          TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT '',
    installed_version TEXT NOT NULL DEFAULT '',
    fixed_version     TEXT NOT NULL DEFAULT '',
    title             TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (isj_namespace, isj_name)
        REFERENCES scan_reports (isj_namespace, isj_name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_scan_report_rows_isj
    ON scan_report_rows (isj_namespace, isj_name);
CREATE INDEX IF NOT EXISTS idx_scan_report_rows_severity
    ON scan_report_rows (severity);
`

// openDB reads KORSAIR_DB_* env vars, dials Postgres (with retry), and
// applies the idempotent schema. Returns the connected DB handle.
func openDB(ctx context.Context) (*sql.DB, error) {
	host := os.Getenv("KORSAIR_DB_HOST")
	port := defaultStr(os.Getenv("KORSAIR_DB_PORT"), "5432")
	user := os.Getenv("KORSAIR_DB_USER")
	pass := os.Getenv("KORSAIR_DB_PASSWORD")
	name := os.Getenv("KORSAIR_DB_NAME")
	ssl := defaultStr(os.Getenv("KORSAIR_DB_SSLMODE"), "disable")

	if host == "" || user == "" || name == "" {
		return nil, fmt.Errorf("KORSAIR_DB_HOST/USER/NAME must be set")
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, name, ssl)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Retry ping for up to ~30s to absorb Postgres pod start-up lag.
	var pingErr error
	for range 15 {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		pingErr = db.PingContext(pctx)
		cancel()
		if pingErr == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if pingErr != nil {
		return nil, fmt.Errorf("ping db: %w", pingErr)
	}

	if _, err := db.ExecContext(ctx, schemaDDL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return db, nil
}

func defaultStr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
