# Examples — Local Dev Environment

This directory contains everything needed to spin up a clean two-cluster local environment for developing and testing Korsair.

## Clusters

| Cluster | Kind name | Context | Role |
|---------|-----------|---------|------|
| Hub | `bly-hub-cluster` | `kind-bly-hub-cluster` | Korsair Operator runs here; stores all scan results |
| Dev (target) | `korsair-dev` | `kind-korsair-dev` | Workloads to be discovered and scanned |

## Quick Start

```sh
./examples/provision.sh
```

This single command:
1. Tears down any existing `bly-hub-cluster` / `korsair-dev` kind clusters
2. Creates both clusters fresh
3. Deploys an `nginx:1.25.3` workload to the dev cluster (namespace `demo`)

### What to do after provisioning

```sh
# 1. Deploy Korsair to the hub cluster
make dev-setup

# 2. Register the dev cluster as a scan target
./hack/add-cluster.sh korsair-dev ~/.kube/config

# 3. Start a scan
kubectl --context kind-bly-hub-cluster apply -f config/samples/security_v1alpha1_securityscanconfig.yaml

# 4. Watch ImageScanJobs appear
kubectl --context kind-bly-hub-cluster get imagescanjobs -A -w
```

## Directory Layout

```
examples/
├── provision.sh              # One-shot bootstrap script
├── clusters/
│   ├── kind-bly-hub-cluster.yaml   # Kind config for the hub cluster
│   └── kind-korsair-dev.yaml       # Kind config for the dev/target cluster
└── workloads/                      # Dynamic sample workloads (auto-loaded by Tilt)
    ├── nginx-demo.yaml             # Sample nginx deployment (namespace: demo)
    ├── redis-demo.yaml             # Sample redis deployment (namespace: demo)
    └── postgres-demo.yaml          # Sample postgres deployment (namespace: demo)
```

### Adding New Workloads

Simply create a new YAML file in `examples/workloads/` — it will automatically be deployed when running:

```sh
tilt up      # Auto-deploys all YAML files in examples/workloads/
```

No need to modify Tiltfile! This makes the workflow modular and extensible.

## Teardown

```sh
kind delete cluster --name bly-hub-cluster
kind delete cluster --name korsair-dev
```
