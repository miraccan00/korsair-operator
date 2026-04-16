# /mission

Context about Korsair Operator's purpose and goals.

## Problem
Production clusters accumulate container images deployed via CI/CD that are never re-scanned. These images passed a scan gate at deploy time but new CVEs have been disclosed since. Point-in-time CI scans miss this class of vulnerability.

## Solution
Korsair continuously re-scans every running image in a cluster (or fleet), deduplicated by digest, using multiple independent scanners (Trivy, Grype). New CVEs against deployed images are surfaced before they become incidents.

## Open-Source Positioning
- Fully open-source, no proprietary dependencies.
- Public CRD schemas and CSV report format — stable contracts for integrations.
- Designed to be deployed by any organization via Helm.

## Enterprise Roadmap
SIEMs, GRC tools, and developer portals will integrate via `API_KEY` to invoke Trivy/Grype through the Korsair API without deploying their own scanning infrastructure (scan-as-a-service).

## Non-Goals
- Not a CI/CD gate (that's existing tooling).
- Not an image registry scanner (scans running workloads, not stored images).
- Not a policy engine yet (ScanPolicy CRD is a future stub).
