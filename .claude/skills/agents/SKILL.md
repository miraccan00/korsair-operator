# /agents

Decompose large Korsair tasks into parallel sub-agents for faster turnaround.

## How Sub-Agents Work in Claude Code

Sub-agents are **separate agent instances** spawned by the main agent via the `Agent` tool.
They run in parallel, each with their own context, then report back a single result.

Skills (`.claude/skills/*/SKILL.md`) are NOT sub-agents — they expand into prompts in
the SAME agent context. Skills can be used to **brief** sub-agents, not replace them.

## Built-in Agent Types

| Type | Use for | Tools available |
|------|---------|----------------|
| `Explore` | Reading code, searching, answering questions about the codebase | Read, Glob, Grep, Bash (read-only) |
| `Plan` | Designing implementation strategies, identifying trade-offs | Read, Glob, Grep |
| `general-purpose` | Multi-step tasks including file writes | All tools |

## Korsair-Specific Decomposition Patterns

### Pattern 1: Feature spanning multiple domains
```
Task: "Add per-team scan quota and routing"

Parallel sub-agents:
├── Explore: "Read api/v1alpha1/securityscanconfig_types.go and
│            internal/controller/securityscanconfig_controller.go.
│            Explain the current discovery fan-out and quota model."
│
├── Explore: "Read cmd/web/db.go and charts/korsair/values.yaml.
│            Explain the PostgreSQL schema and Helm configuration."
│
└── Plan:    "Design CRD changes to SecurityScanConfig for per-team quota
             and NotificationPolicy routing. Consider public contract stability."

Main agent: synthesizes results → implements
```

### Pattern 2: Investigating a CI failure
```
Task: "CI lint is failing on 3 files"

Parallel sub-agents:
├── Explore: "Read cmd/web/main.go lines 260-380. Find errcheck, lll violations."
├── Explore: "Read internal/controller/imagescanresult_controller.go lines 710-780."
└── Explore: "Read cmd/web/api_test.go. Find defer errcheck issues."

Main agent: applies all fixes in one pass
```

### Pattern 3: Cross-cutting refactor
```
Task: "Rename ImageScanJob → ScanResult across the codebase"

Sequential (cannot parallelize writes):
1. Explore agent: "Find all occurrences of ImageScanJob in Go source, CRDs, Helm"
2. Plan agent: "Plan rename sequence to avoid compilation errors mid-refactor"
3. Main agent: executes rename in dependency order
```

## When to Use Sub-Agents

**Spawn sub-agents when:**
- Task requires reading 3+ unrelated file domains simultaneously
- Research and planning can proceed without blocking each other
- You need an independent code review (fresh context, no anchoring bias)

**Do inline when:**
- Single-file edit or bug fix
- File content is already in context
- Task is sequential by nature (each step depends on the previous)

## Decomposition Template

When the user gives a large prompt, respond with:

```
I'll break this into parallel research tasks:

[Explore agent 1] → {specific files + specific question}
[Explore agent 2] → {specific files + specific question}
[Plan agent]      → {design question with context from above}

Then synthesize and implement.
```

## Korsair Domain Map (for briefing sub-agents)

| Domain | Key files |
|--------|-----------|
| CRD types | `api/v1alpha1/*_types.go` |
| Operator reconcilers | `internal/controller/*_controller.go` |
| API server + DB | `cmd/web/main.go`, `cmd/web/db.go` |
| Helm values | `charts/korsair/values.yaml` |
| Frontend | `frontend/src/` |
| CI workflows | `.github/workflows/` |
| Tests | `internal/controller/*_test.go`, `cmd/web/api_test.go` |
