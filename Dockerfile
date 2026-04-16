# ── Stage 1: In-container source build (CI / reproducible releases) ──────────
#
# Used by:  docker build --target runtime-source -t korsair-operator:latest .
#           make docker-build / make docker-build-all
#
# BuildKit cache mounts keep module downloads and the Go build cache between
# runs so that only changed packages are recompiled.
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -o /out/manager ./cmd

# ── Stage 2a: Runtime image built from source ─────────────────────────────────
FROM gcr.io/distroless/static:nonroot AS runtime-source
COPY --from=builder /out/manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]

# ── Stage 2b: Runtime image from a host-built binary (fast Tilt iteration) ────
#
# Used by:  docker build --target runtime-prebuilt -t korsair-operator:dev .
#           Tilt build-operator resource (go build on host → ~5s docker build)
#
# Requires bin/manager to exist (produced by the Tilt host go build step).
# bin/ is intentionally NOT listed in .dockerignore for this reason.
FROM gcr.io/distroless/static:nonroot AS runtime-prebuilt
COPY bin/manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
