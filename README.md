# Karpenter Provider for DigitalOcean

[![CI](https://github.com/synthesizer/karpenter-provider-digitalocean/actions/workflows/ci.yaml/badge.svg)](https://github.com/synthesizer/karpenter-provider-digitalocean/actions/workflows/ci.yaml)

A [Karpenter](https://karpenter.sh/) provider that enables automatic node provisioning on [DigitalOcean](https://www.digitalocean.com/) using Droplets.

Karpenter is a Kubernetes node autoscaler built for flexibility, performance, and simplicity. This provider extends Karpenter to provision DigitalOcean Droplets as Kubernetes nodes, automatically selecting instance types based on pending pod requirements and removing nodes when they are no longer needed.

## Features

- **Automatic node provisioning** — schedules pending pods by creating right-sized DigitalOcean Droplets
- **Instance type selection** — maps DigitalOcean Droplet sizes (Basic, General Purpose, CPU-Optimized, Memory-Optimized, Storage-Optimized, GPU) to Karpenter instance types
- **Drift detection** — detects when running nodes drift from their desired `DONodeClass` configuration
- **Node disruption** — supports consolidation, expiration, and voluntary disruption via Karpenter core
- **Garbage collection** — cleans up orphaned Droplets that no longer have a corresponding `NodeClaim`
- **VPC placement** — places Droplets in a specified VPC or the region's default VPC
- **Load balancer integration** — adds/removes Droplets from DigitalOcean Load Balancers
- **Custom images** — supports both DigitalOcean image slugs and custom snapshot IDs
- **Cloud-init / user data** — bootstraps nodes into the cluster via cloud-init scripts

## Architecture

```
┌──────────────────────────────────────────────────┐
│  Karpenter Core (sigs.k8s.io/karpenter)          │
│  ┌──────────┐ ┌───────────┐ ┌─────────────────┐  │
│  │Provision │ │ Disrupt   │ │ Node Lifecycle  │  │
│  └────┬─────┘ └─────┬─────┘ └────────┬────────┘  │
│       │             │                │            │
│       └─────────────┼────────────────┘            │
│                     ▼                             │
│           CloudProvider Interface                 │
└─────────────────────┬────────────────────────────┘
                      │
┌─────────────────────▼────────────────────────────┐
│  DigitalOcean Provider                            │
│  ┌──────────┐ ┌──────────────┐ ┌───────┐         │
│  │ Instance │ │ InstanceType │ │ Image │         │
│  └──────────┘ └──────────────┘ └───────┘         │
│  ┌──────────┐ ┌──────────────┐ ┌───────┐         │
│  │ Pricing  │ │    Region    │ │  VPC  │         │
│  └──────────┘ └──────────────┘ └───────┘         │
│  ┌──────────────────┐                             │
│  │  Load Balancer   │                             │
│  └──────────────────┘                             │
│                      │                            │
│                DigitalOcean API (godo)             │
└──────────────────────────────────────────────────┘
```

## Prerequisites

- **Go 1.24+** (for building from source)
- **Kubernetes 1.27+** cluster on DigitalOcean (DOKS or self-managed)
- **Helm 3** (for chart-based installation)
- **DigitalOcean API token** with read/write access to Droplets, Images, Regions, VPCs, and Load Balancers
- **kubectl** configured to access your cluster

## Quick Start

### 1. Create a DigitalOcean API Token

Create a personal access token in the [DigitalOcean Control Panel](https://cloud.digitalocean.com/account/api/tokens) with full read/write scope.

### 2. Install with Helm

```bash
helm upgrade --install karpenter-do charts/karpenter-do/ \
  --namespace kube-system \
  --create-namespace \
  --set settings.clusterName=my-cluster \
  --set settings.clusterEndpoint=https://my-cluster-api-endpoint:6443 \
  --set digitalocean.accessToken=dop_v1_xxxxxxxxxxxx
```

Or use an existing Kubernetes secret:

```bash
# Create the secret first
kubectl create secret generic do-token \
  --namespace kube-system \
  --from-literal=access-token=dop_v1_xxxxxxxxxxxx

# Install referencing the existing secret
helm upgrade --install karpenter-do charts/karpenter-do/ \
  --namespace kube-system \
  --set settings.clusterName=my-cluster \
  --set settings.clusterEndpoint=https://my-cluster-api-endpoint:6443 \
  --set digitalocean.existingSecret=do-token
```

### 3. Create a DONodeClass

The `DONodeClass` defines DigitalOcean-specific configuration for nodes:

```yaml
apiVersion: karpenter.do.sh/v1alpha1
kind: DONodeClass
metadata:
  name: default
spec:
  region: nyc1
  image:
    slug: ubuntu-24-04-x64
  sshKeys:
    - "ab:cd:ef:..."       # SSH key fingerprint
  tags:
    - my-app
  # Optional: specify a VPC (defaults to the region's default VPC)
  # vpcUUID: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  # Optional: cloud-init script to bootstrap the node
  # userData: |
  #   #!/bin/bash
  #   curl -sfL https://example.com/bootstrap.sh | sh -
```

Or use a custom snapshot:

```yaml
apiVersion: karpenter.do.sh/v1alpha1
kind: DONodeClass
metadata:
  name: custom-image
spec:
  region: sfo3
  image:
    id: 123456789   # Custom snapshot ID
```

### 4. Create a NodePool

The `NodePool` tells Karpenter what kinds of nodes to provision and references a `DONodeClass`:

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  template:
    spec:
      nodeClassRef:
        group: karpenter.do.sh
        kind: DONodeClass
        name: default
      requirements:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
        - key: karpenter.do.sh/instance-type-family
          operator: In
          values: ["s", "g", "c"]  # Basic, General Purpose, CPU-Optimized
        - key: node.kubernetes.io/instance-type
          operator: In
          values: ["s-2vcpu-4gb", "s-4vcpu-8gb", "g-2vcpu-8gb"]
  limits:
    cpu: "100"
    memory: 400Gi
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 1m
```

### 5. Deploy a Workload

Deploy a workload and watch Karpenter provision nodes:

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: inflate
spec:
  replicas: 5
  selector:
    matchLabels:
      app: inflate
  template:
    metadata:
      labels:
        app: inflate
    spec:
      containers:
        - name: inflate
          image: public.ecr.aws/eks-distro/kubernetes/pause:3.7
          resources:
            requests:
              cpu: "1"
              memory: 1Gi
EOF
```

Watch the nodes being created:

```bash
kubectl get nodeclaims -w
kubectl get nodes -w
```

## Building from Source

### Build the Binary

```bash
make build
```

This produces `bin/karpenter-do`.

### Build the Docker Image

```bash
# Build for the local platform
make docker-build

# Build with a custom tag
IMG=my-registry/karpenter-do:dev make docker-build

# Push to a registry
IMG=my-registry/karpenter-do:dev make docker-push
```

### Run Locally (for Development)

```bash
export DIGITALOCEAN_ACCESS_TOKEN=dop_v1_xxxxxxxxxxxx
export CLUSTER_NAME=my-cluster
export CLUSTER_ENDPOINT=https://my-cluster:6443
export KUBECONFIG=~/.kube/config

make run
```

## Development

### Project Structure

```
├── cmd/controller/         # Main entry point
├── pkg/
│   ├── apis/v1alpha1/      # DONodeClass CRD types
│   ├── cloudprovider/      # Karpenter CloudProvider implementation
│   ├── controllers/
│   │   ├── nodeclass/      # DONodeClass reconciler
│   │   └── nodeclaim/      # NodeClaim garbage collection
│   ├── fake/               # Test mocks and fixtures
│   ├── operator/           # DigitalOcean operator (provider init)
│   └── providers/
│       ├── image/          # OS image resolution
│       ├── instance/       # Droplet lifecycle (CRUD)
│       ├── instancetype/   # Droplet size → Karpenter InstanceType
│       ├── loadbalancer/   # LB ↔ Droplet membership
│       ├── pricing/        # Hourly pricing data
│       ├── region/         # Region availability
│       └── vpc/            # VPC lookup
├── charts/karpenter-do/    # Helm chart
├── designs/                # Design documents
├── hack/                   # Code generation boilerplate
└── test/                   # Integration and E2E test suites
```

### Run Tests

```bash
# Unit tests
make test

# Unit tests with verbose output and race detection
go test ./pkg/... -v -race

# Integration tests (requires DO credentials)
make test-integration

# E2E tests (requires a running cluster + DO credentials)
make test-e2e
```

### Lint

```bash
# Requires golangci-lint v1.64+
make lint
```

### Code Generation

After modifying types in `pkg/apis/v1alpha1/types.go`, regenerate deep-copy methods and CRDs:

```bash
make generate
```

### Format and Vet

```bash
make fmt
make vet
```

### Verify Everything

Run all checks (generate, format, vet) and ensure no uncommitted diffs:

```bash
make verify
```

## Configuration

### Environment Variables

| Variable | Required | Description |
|---|---|---|
| `DIGITALOCEAN_ACCESS_TOKEN` | Yes | DigitalOcean API token |
| `CLUSTER_NAME` | Yes | Name of the Kubernetes cluster |
| `CLUSTER_ENDPOINT` | Yes | Kubernetes API server endpoint |

### Helm Values

| Key | Default | Description |
|---|---|---|
| `replicaCount` | `2` | Number of controller replicas |
| `image.repository` | `digitalocean/karpenter-provider-do` | Container image repository |
| `image.tag` | Chart `appVersion` | Container image tag |
| `settings.clusterName` | `""` | Kubernetes cluster name |
| `settings.clusterEndpoint` | `""` | Kubernetes API endpoint |
| `digitalocean.accessToken` | `""` | DO API token (creates a Secret) |
| `digitalocean.existingSecret` | `""` | Name of an existing Secret |
| `digitalocean.secretKey` | `access-token` | Key inside the Secret |
| `resources.requests.cpu` | `100m` | CPU request |
| `resources.requests.memory` | `128Mi` | Memory request |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `512Mi` | Memory limit |
| `webhook.enabled` | `false` | Enable admission webhook |
| `metrics.enabled` | `true` | Enable Prometheus metrics |
| `metrics.port` | `8080` | Metrics server port |
| `health.port` | `8081` | Health check port |

## DONodeClass Reference

### Spec

| Field | Type | Required | Description |
|---|---|---|---|
| `region` | `string` | Yes | DigitalOcean region slug (`nyc1`, `sfo3`, `ams3`, etc.) |
| `image.slug` | `string` | One of `slug` or `id` | DO image slug (`ubuntu-24-04-x64`) |
| `image.id` | `int` | One of `slug` or `id` | Custom image or snapshot ID |
| `vpcUUID` | `string` | No | VPC UUID; defaults to region's default VPC |
| `sshKeys` | `[]string` | No | SSH key fingerprints or IDs |
| `tags` | `[]string` | No | Additional tags for Droplets |
| `userData` | `string` | No | Cloud-init / bootstrap script |
| `blockStorage.sizeGiB` | `int` | No | Block storage volume size (1–16384 GiB) |
| `blockStorage.fsType` | `string` | No | Filesystem type (default: `ext4`) |

### Status

| Field | Description |
|---|---|
| `imageID` | Resolved DigitalOcean image ID |
| `specHash` | Hash of the spec for drift detection |
| `conditions` | Standard Kubernetes conditions (`Ready`, `ImageResolved`, `VPCValid`) |

### Labels Set on Nodes

| Label | Example | Description |
|---|---|---|
| `karpenter.do.sh/instance-type-family` | `s`, `g`, `c`, `m` | Droplet family |
| `karpenter.do.sh/instance-size` | `s-2vcpu-4gb` | Full size slug |
| `karpenter.do.sh/region` | `nyc1` | Region |
| `karpenter.do.sh/image-id` | `12345` | Resolved image ID |
| `karpenter.do.sh/vpc-uuid` | `uuid-here` | VPC UUID |

## CI/CD

The project uses GitHub Actions with the following workflows:

| Workflow | Trigger | Description |
|---|---|---|
| **CI** (`ci.yaml`) | Push to `main`, PRs | Lint, unit tests, build, Docker build, verify generated code |
| **Integration** (`integration.yaml`) | Manual, nightly | Integration tests against live DigitalOcean infrastructure |
| **Release** (`release.yaml`) | `v*` tags | Multi-arch Docker images, cross-compiled binaries, GitHub Release |

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes and ensure tests pass (`make test lint`)
4. Commit and push (`git push origin feature/my-feature`)
5. Open a Pull Request

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
