# /changelog-generator

Generate a changelog entry for Korsair Operator.

## Format (Keep-a-Changelog)
```markdown
## [Unreleased] or ## [vX.Y.Z] - YYYY-MM-DD

### Added
### Changed
### Fixed
### Deprecated
### Removed
```

## Steps
1. Run `git log --oneline` since last tag to find commits.
2. Group by type: feat→Added, fix→Fixed, refactor→Changed, etc.
3. For each entry, reference the affected CRD or component: `[ImageScanJob]`, `[API]`, `[UI]`, `[Helm]`.
4. Flag any breaking changes to CSV schema, CRD fields, or API responses with **BREAKING**.

## Rules
- No internal references or usernames.
- Scanner version bumps go under Changed with old→new format.
- CRD schema changes that are additive go under Changed; removals/renames are BREAKING.
