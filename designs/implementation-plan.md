# Karpenter Provider DigitalOcean — Implementation Plan

## Overview

This document outlines the implementation plan for `karpenter-provider-digitalocean`, a [Karpenter](https://github.com/kubernetes-sigs/karpenter) cloud provider that enables Kubernetes node autoscaling on [DigitalOcean](https://www.digitalocean.com/).

Karpenter watches for unschedulable pods, evaluates scheduling constraints, provisions nodes that meet those constraints, and removes nodes when they are no longer needed. This provider translates those operations into DigitalOcean Droplet lifecycle management.

### Reference Implementations

- [karpenter-provider-aws](https://github.com/aws/karpenter-provider-aws) — canonical, most mature
- [karpenter-provider-azure](https://github.com/Azure/karpenter-provider-azure)
- [karpenter-provider-gcp](https://github.com/kubernetes-sigs/karpenter-provider-gcp)
- [karpenter-provider-cluster-api](https://github.com/kubernetes-sigs/karpenter-provider-cluster-api)

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                    karpenter-provider-digitalocean               │
│                                                                  │
│  ┌──────────────┐   ┌──────────────────────────────────────────┐│
│  │   Operator    │   │           CloudProvider                  ││
│  │              │   │                                          ││
│  │  - Injects   │   │  Create(ctx, NodeClaim) → NodeClaim     ││
│  │    DO client │   │  Delete(ctx, NodeClaim) → error          ││
│  │  - Registers │   │  Get(ctx, providerID) → NodeClaim        ││
│  │    schemes   │   │  List(ctx) → []NodeClaim                 ││
│  │  - Starts    │   │  GetInstanceTypes(ctx, NodePool) → []IT  ││
│  │    providers │   │  GetSupportedNodeClasses() → []Object    ││
│  │              │   │  IsDrifted(ctx, NodeClaim) → DriftReason ││
│  └──────┬───────┘   └──────────────┬───────────────────────────┘│
│         │                          │                             │
│  ┌──────▼──────────────────────────▼───────────────────────────┐│
│  │                     Providers                                ││
│  │                                                              ││
│  │  ┌──────────┐ ┌──────────────┐ ┌─────────┐ ┌─────────────┐ ││
│  │  │ Instance │ │ InstanceType │ │  Image  │ │   Pricing   │ ││
│  │  │ Provider │ │   Provider   │ │Provider │ │  Provider   │ ││
│  │  └────┬─────┘ └──────┬───────┘ └────┬────┘ └──────┬──────┘ ││
│  │       │               │              │             │         ││
│  │  ┌────▼───┐    ┌──────▼──────┐  ┌───▼────┐ ┌─────▼──────┐ ││
│  │  │Region  │    │     VPC     │  │  LB    │ │            │ ││
│  │  │Provider│    │   Provider  │  │Provider│ │            │ ││
│  │  └────────┘    └─────────────┘  └────────┘ └────────────┘ ││
│  └──────────────────────────────────────────────────────────────┘│
│                              │                                   │
│                    ┌─────────▼──────────┐                       │
│                    │  DigitalOcean API   │                       │
│                    │  (godo Go SDK)      │                       │
│                    └────────────────────┘                       │
└──────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Project Foundation

### 1.1 Go Module & Dependencies

**File:** `go.mod`

```
module github.com/digitalocean/karpenter-provider-digitalocean

go 1.23

require (
    sigs.k8s.io/karpenter v1.1.x
    github.com/digitalocean/godo v1.x.x
    k8s.io/api ...
    k8s.io/apimachinery ...
    k8s.io/client-go ...
    sigs.k8s.io/controller-runtime ...
)
```

Key dependencies:
- **`sigs.k8s.io/karpenter`** — core karpenter library (CloudProvider interface, scheduling, disruption)
- **`github.com/digitalocean/godo`** — DigitalOcean Go API client
- **`sigs.k8s.io/controller-runtime`** — controller framework
- **`k8s.io/*`** — Kubernetes API types

### 1.2 Build Infrastructure

| File | Purpose |
|------|---------|
| `Makefile` | Build, test, generate, lint targets |
| `Dockerfile` | Multi-stage build for controller binary |
| `.golangci.yaml` | Linter configuration |
| `hack/toolchain.sh` | Install dev tooling (controller-gen, envtest, etc.) |
| `.github/workflows/ci.yaml` | CI pipeline (lint, test, build, e2e) |

---

## Phase 2: Custom Resource Definitions (CRDs)

### 2.1 DONodeClass (`pkg/apis/v1alpha1/`)

The `DONodeClass` is the DigitalOcean-specific NodeClass resource, referenced by Karpenter's `NodePool` via `spec.template.spec.nodeClassRef`.

```yaml
apiVersion: karpenter.do.sh/v1alpha1
kind: DONodeClass
metadata:
  name: default
spec:
  # DigitalOcean region (e.g., "nyc1", "sfo3", "ams3")
  region: nyc1

  # VPC UUID — droplets will be placed in this VPC
  vpcUUID: "abc-123-..."

  # Image configuration for worker nodes
  image:
    # OS slug or custom image ID
    slug: "ubuntu-24-04-x64"
    # OR custom snapshot ID
    # id: 12345678

  # SSH key fingerprints/IDs to add to droplets
  sshKeys:
    - "ab:cd:ef:..."

  # Tags applied to all droplets managed by this NodeClass
  tags:
    - "karpenter"
    - "k8s-worker"

  # User data script (cloud-init) for node bootstrap
  # Typically joins the node to the Kubernetes cluster
  userData: |
    #!/bin/bash
    # Bootstrap script to join the K8s cluster
    ...

  # Block storage volumes to attach
  blockStorage:
    sizeGiB: 100
    fsType: ext4

status:
  # Resolved image ID
  imageID: "12345678"
  # Conditions
  conditions:
    - type: Ready
      status: "True"
```

**Files to create:**

| File | Contents |
|------|----------|
| `pkg/apis/v1alpha1/doc.go` | Package doc, `+groupName=karpenter.do.sh` |
| `pkg/apis/v1alpha1/types.go` | Go types: `DONodeClass`, `DONodeClassSpec`, `DONodeClassStatus` |
| `pkg/apis/v1alpha1/register.go` | Scheme registration |
| `pkg/apis/v1alpha1/zz_generated.deepcopy.go` | Generated deepcopy methods |
| `pkg/apis/v1alpha1/labels.go` | Well-known labels/annotations |
| `pkg/apis/v1alpha1/types_instancetype.go` | InstanceType label constants |

### 2.2 Well-Known Labels

```go
const (
    Group   = "karpenter.do.sh"
    Version = "v1alpha1"

    // DigitalOcean-specific labels
    LabelInstanceTypeFamily = Group + "/instance-type-family"  // e.g., "s", "g", "m", "c", "so"
    LabelInstanceSize       = Group + "/instance-size"         // e.g., "s-1vcpu-2gb"
    LabelRegion             = Group + "/region"                // e.g., "nyc1"
    LabelImageID            = Group + "/image-id"
    LabelVPCUUID            = Group + "/vpc-uuid"

    // DO droplet size families:
    // s-*   = Basic (shared CPU)
    // g-*   = General Purpose (dedicated CPU)
    // c-*   = CPU-Optimized
    // m-*   = Memory-Optimized
    // so-*  = Storage-Optimized
    // gd-*  = General Purpose + SSD
    // gpu-* = GPU Droplets
)
```

---

## Phase 3: CloudProvider Interface Implementation

### 3.1 Interface Contract

The `CloudProvider` interface from `sigs.k8s.io/karpenter/pkg/cloudprovider` requires:

```go
type CloudProvider interface {
    // Create launches a new instance given a NodeClaim spec
    Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error)

    // Delete terminates an instance
    Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error

    // Get returns a NodeClaim for a known provider ID
    Get(ctx context.Context, providerID string) (*v1.NodeClaim, error)

    // List returns all instances managed by this provider
    List(ctx context.Context) ([]*v1.NodeClaim, error)

    // GetInstanceTypes returns available instance types for the given NodePool
    GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*cloudprovider.InstanceType, error)

    // GetSupportedNodeClasses returns the GVKs this provider supports
    GetSupportedNodeClasses() []status.Object

    // IsDrifted checks if a NodeClaim has drifted from its desired state
    IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (cloudprovider.DriftReason, error)
}
```

### 3.2 Implementation File: `pkg/cloudprovider/cloudprovider.go`

```go
type CloudProvider struct {
    instanceTypeProvider instancetype.Provider
    instanceProvider     instance.Provider
    imageProvider        image.Provider
    kubeClient           client.Client
}
```

**Method Mapping to DigitalOcean:**

| Method | DO API Calls | Notes |
|--------|-------------|-------|
| `Create` | `godo.Droplets.Create()` | Create droplet with resolved size, image, region, VPC, userData |
| `Delete` | `godo.Droplets.Delete()` | Delete by droplet ID from providerID |
| `Get` | `godo.Droplets.Get()` | Parse providerID → droplet ID, fetch status |
| `List` | `godo.Droplets.ListByTag()` | List all karpenter-managed droplets |
| `GetInstanceTypes` | Cached: `godo.Sizes.List()` | Map DO sizes to `cloudprovider.InstanceType` |
| `GetSupportedNodeClasses` | Static | Returns `[]DONodeClass{}` |
| `IsDrifted` | Compare current vs desired | Image, region, VPC, size family changes |

### 3.3 Provider ID Format

Following Kubernetes conventions, the provider ID for DigitalOcean nodes:

```
digitalocean://<region>/<droplet-id>
```

Example: `digitalocean://nyc1/12345678`

---

## Phase 4: Provider Implementations

### 4.1 Instance Provider (`pkg/providers/instance/`)

Manages Droplet lifecycle.

```go
type Provider interface {
    Create(ctx context.Context, nodeClass *v1alpha1.DONodeClass, nodeClaim *v1.NodeClaim,
           instanceTypes []*cloudprovider.InstanceType) (*Instance, error)
    Delete(ctx context.Context, id string) error
    Get(ctx context.Context, id string) (*Instance, error)
    List(ctx context.Context) ([]*Instance, error)
}

type Instance struct {
    ID         int
    Name       string
    Region     string
    Size       string
    Status     string
    PrivateIP  string
    PublicIP   string
    Tags       []string
    CreatedAt  time.Time
    ImageID    int
    VPCUUID    string
}
```

**Key Implementation Details:**
- Create droplet with `godo.DropletCreateRequest`
- Set `UserData` from DONodeClass (cloud-init bootstrap script)
- Tag droplets with `karpenter.do.sh/managed=true`, `karpenter.do.sh/nodepool=<name>`
- Parse scheduling requirements to select appropriate DO size
- Wait for droplet to reach `active` status
- Resolve private IP for node registration

### 4.2 Instance Type Provider (`pkg/providers/instancetype/`)

Maps DigitalOcean Droplet sizes to Karpenter's `InstanceType` abstraction.

```go
type Provider interface {
    List(ctx context.Context, nodeClass *v1alpha1.DONodeClass) ([]*cloudprovider.InstanceType, error)
    // Cached — refreshes periodically from DO API
}
```

**DigitalOcean Size → Karpenter InstanceType Mapping:**

| DO Size Slug | vCPUs | Memory | Disk | Family | Price/hr |
|-------------|-------|--------|------|--------|----------|
| `s-1vcpu-1gb` | 1 | 1 GiB | 25 GB | Basic | $0.00893 |
| `s-1vcpu-2gb` | 1 | 2 GiB | 50 GB | Basic | $0.01786 |
| `s-2vcpu-4gb` | 2 | 4 GiB | 80 GB | Basic | $0.03571 |
| `s-4vcpu-8gb` | 4 | 8 GiB | 160 GB | Basic | $0.07143 |
| `g-2vcpu-8gb` | 2 | 8 GiB | 25 GB | General | $0.09375 |
| `c-2vcpu-4gb` | 2 | 4 GiB | 25 GB | CPU-Opt | $0.06250 |
| `m-2vcpu-16gb` | 2 | 16 GiB | 50 GB | Mem-Opt | $0.12500 |
| `gpu-h100x1-80gb` | 20 | 240 GiB | 50 GB | GPU | varies |

Each size becomes a `cloudprovider.InstanceType` with:
- `Name`: the DO size slug
- `Requirements`: vCPUs, memory, architecture (amd64), capacity type (on-demand)
- `Offerings`: availability per region + pricing
- `Capacity`: allocatable resources (after system overhead)
- `Overhead`: reserved for kubelet, system daemons

### 4.3 Image Provider (`pkg/providers/image/`)

Resolves OS images for worker nodes.

```go
type Provider interface {
    // Resolve returns the image ID for the given NodeClass
    Resolve(ctx context.Context, nodeClass *v1alpha1.DONodeClass) (int, error)
}
```

**Strategies:**
1. **Slug-based**: Use a well-known DO image slug (e.g., `ubuntu-24-04-x64`)
2. **Custom snapshot**: Use a pre-built custom image with Kubernetes components
3. **DOKS-compatible**: Use the same image DigitalOcean uses for DOKS worker nodes (preferred, if accessible)

### 4.4 Pricing Provider (`pkg/providers/pricing/`)

Provides instance pricing for Karpenter's cost-aware scheduling and consolidation.

```go
type Provider interface {
    // InstanceTypePrice returns the hourly price for a size in a region
    InstanceTypePrice(sizeName, region string) (float64, bool)
    // LivePricing refreshes prices from the DO API
    LivePricing(ctx context.Context) error
}
```

**Data Sources:**
- `godo.Sizes.List()` returns `PriceHourly` and `PriceMonthly` per size
- Cache prices with periodic refresh (every 12-24 hours)
- DigitalOcean only has on-demand pricing (no spot/preemptible)

### 4.5 Region Provider (`pkg/providers/region/`)

```go
type Provider interface {
    List(ctx context.Context) ([]string, error)
    IsAvailable(ctx context.Context, region string) bool
}
```

### 4.6 VPC Provider (`pkg/providers/vpc/`)

```go
type Provider interface {
    Get(ctx context.Context, id string) (*godo.VPC, error)
    GetDefault(ctx context.Context, region string) (*godo.VPC, error)
}
```

### 4.7 Load Balancer Provider (`pkg/providers/loadbalancer/`)

Optional — tracks load balancers to ensure new nodes are registered.

```go
type Provider interface {
    // AddDroplets registers droplets with relevant load balancers
    AddDroplets(ctx context.Context, lbID string, dropletIDs ...int) error
    RemoveDroplets(ctx context.Context, lbID string, dropletIDs ...int) error
}
```

---

## Phase 5: Controllers

### 5.1 NodeClass Controller (`pkg/controllers/nodeclass/`)

Watches `DONodeClass` resources and:
- Resolves image IDs from slugs/snapshots
- Validates VPC existence
- Updates `status.conditions`
- Sets `status.imageID` after resolution

### 5.2 NodeClaim Controller (`pkg/controllers/nodeclaim/`)

Extends core Karpenter's NodeClaim reconciliation with DO-specific:
- Garbage collection of orphaned droplets
- Status synchronization (droplet status → NodeClaim status)
- Tagging/labeling droplet metadata

---

## Phase 6: Operator & Controller Manager

### 6.1 Operator (`pkg/operator/`)

```go
type Operator struct {
    *operator.Operator  // embeds core karpenter operator

    DOClient        *godo.Client
    ImageProvider   image.Provider
    InstanceProvider instance.Provider
    // ... other providers
}
```

Sets up:
- DigitalOcean API client (from `DIGITALOCEAN_ACCESS_TOKEN` env var or secret)
- All provider instances with appropriate caching
- Scheme registration for `DONodeClass`

### 6.2 Controller Entry Point (`cmd/controller/main.go`)

```go
func main() {
    // 1. Initialize core karpenter operator
    // 2. Create DO-specific operator (wraps core)
    // 3. Register cloud provider
    // 4. Start controller manager
}
```

---

## Phase 7: Helm Chart

### 7.1 Chart Structure (`charts/karpenter-do/`)

```
charts/karpenter-do/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── deployment.yaml        # Karpenter controller
│   ├── serviceaccount.yaml
│   ├── clusterrole.yaml       # RBAC for NodeClaim, DONodeClass, Nodes, Pods
│   ├── clusterrolebinding.yaml
│   ├── secret.yaml            # DO API token
│   ├── configmap.yaml         # Controller settings
│   └── crds/
│       └── donodeclass.yaml   # DONodeClass CRD
```

### 7.2 Key Configuration Values

```yaml
# values.yaml
replicas: 2
image:
  repository: digitalocean/karpenter-provider-do
  tag: v0.1.0

settings:
  clusterName: my-cluster
  clusterEndpoint: https://xxx.k8s.ondigitalocean.com

digitalocean:
  # Provided via secret or direct value
  accessToken: ""
  # OR reference an existing secret
  existingSecret: ""
  secretKey: "access-token"

  region: nyc1
  vpcUUID: ""

serviceAccount:
  create: true
  name: karpenter
```

---

## Phase 8: Testing Strategy

### 8.1 Unit Tests
- Provider-level unit tests with mocked `godo.Client`
- CloudProvider interface tests
- Instance type mapping tests
- Pricing calculation tests

### 8.2 Integration Tests (`test/suites/integration/`)
- NodeClaim creation → Droplet creation
- NodeClaim deletion → Droplet deletion
- Drift detection and node replacement
- Instance type selection based on pod requirements

### 8.3 E2E Tests (`test/suites/e2e/`)
- Full pod scheduling → node provisioning → pod running flow
- Scale-up and scale-down scenarios
- Consolidation behavior
- Disruption budgets

### 8.4 Fake/Mock Infrastructure (`pkg/fake/`)
- Fake `godo.Client` for deterministic testing
- Fake instance type catalog
- Fake pricing data

---

## DigitalOcean-Specific Considerations

### Node Bootstrap

Since DigitalOcean doesn't have a managed node bootstrap mechanism like AWS's `aws-node-termination-handler` or EKS Bootstrap, we need to handle this via **cloud-init** in the Droplet's `user_data`:

1. Install container runtime (containerd)
2. Install kubelet, kubeadm
3. Retrieve bootstrap token from a secure source
4. `kubeadm join` the cluster
5. Label the node with Karpenter metadata

**Alternative:** Use a pre-baked custom image (snapshot) that has all K8s components pre-installed, reducing bootstrap time.

### DOKS vs. Self-Managed

This provider is designed for **self-managed Kubernetes clusters on DigitalOcean** (not DOKS). DOKS manages its own worker node pools and doesn't expose the ability to add arbitrary droplets as worker nodes.

For DOKS integration, a separate approach using the DO Kubernetes API would be needed (adding node pools via the API), which is a different architecture.

### Capacity Type

DigitalOcean does not offer spot/preemptible instances. All instances are `on-demand`. The provider should:
- Only advertise `on-demand` capacity type
- Not attempt spot-like fallback behavior

### Architecture

DigitalOcean droplets are currently `amd64` only (Intel/AMD x86_64). GPU droplets use NVIDIA GPUs but still have `amd64` CPU architecture.

### Rate Limiting

The DigitalOcean API has rate limits (250 requests/5 min). The provider must:
- Cache aggressively (sizes, regions, images)
- Use tag-based listing instead of individual lookups
- Implement exponential backoff on 429 responses

---

## Implementation Priority / Milestones

### Milestone 1: MVP (Weeks 1-4)
- [ ] Project scaffolding (go.mod, Makefile, Dockerfile)
- [ ] `DONodeClass` CRD types
- [ ] Instance Type Provider (size catalog)
- [ ] Instance Provider (create/delete droplets)
- [ ] CloudProvider interface (Create, Delete, List, Get, GetInstanceTypes)
- [ ] Basic controller wiring
- [ ] Unit tests with fake DO client

### Milestone 2: Core Features (Weeks 5-8)
- [ ] Image Provider (slug & custom image resolution)
- [ ] Pricing Provider
- [ ] Drift detection (`IsDrifted`)
- [ ] NodeClass controller (status reconciliation)
- [ ] VPC Provider
- [ ] Helm chart v1
- [ ] Integration tests

### Milestone 3: Production Readiness (Weeks 9-12)
- [ ] Load Balancer Provider
- [ ] Node bootstrap optimization (custom images)
- [ ] Garbage collection for orphaned droplets
- [ ] Metrics and observability
- [ ] E2E test suite
- [ ] Documentation
- [ ] Security hardening (token rotation, RBAC)

### Milestone 4: Advanced (Weeks 13+)
- [ ] GPU droplet support
- [ ] Block storage integration
- [ ] Consolidation tuning for DO pricing
- [ ] DOKS-mode (manage node pools instead of raw droplets)
- [ ] Topology-aware scheduling (per-region, per-datacenter)

---

## File Inventory

```
karpenter-provider-digitalocean/
├── cmd/
│   └── controller/
│       └── main.go                          # Entry point
├── pkg/
│   ├── apis/
│   │   └── v1alpha1/
│   │       ├── doc.go                       # +groupName marker
│   │       ├── types.go                     # DONodeClass types
│   │       ├── types_instancetype.go        # Instance type constants
│   │       ├── register.go                  # Scheme registration
│   │       ├── labels.go                    # Well-known labels
│   │       └── zz_generated.deepcopy.go     # Generated
│   ├── cloudprovider/
│   │   ├── cloudprovider.go                 # CloudProvider implementation
│   │   └── cloudprovider_test.go
│   ├── controllers/
│   │   ├── nodeclass/
│   │   │   ├── controller.go                # DONodeClass reconciler
│   │   │   └── controller_test.go
│   │   └── nodeclaim/
│   │       ├── controller.go                # DO-specific NodeClaim logic
│   │       └── controller_test.go
│   ├── fake/
│   │   ├── do_client.go                     # Fake godo.Client
│   │   └── instance_types.go                # Fake size catalog
│   ├── operator/
│   │   └── operator.go                      # DO operator setup
│   ├── providers/
│   │   ├── instance/
│   │   │   ├── instance.go                  # Droplet CRUD
│   │   │   ├── types.go                     # Instance types
│   │   │   └── instance_test.go
│   │   ├── instancetype/
│   │   │   ├── instancetype.go              # Size → InstanceType mapping
│   │   │   ├── offerings.go                 # Regional availability + pricing
│   │   │   └── instancetype_test.go
│   │   ├── image/
│   │   │   ├── image.go                     # Image resolution
│   │   │   └── image_test.go
│   │   ├── pricing/
│   │   │   ├── pricing.go                   # DO pricing data
│   │   │   └── pricing_test.go
│   │   ├── region/
│   │   │   └── region.go                    # Region availability
│   │   ├── vpc/
│   │   │   └── vpc.go                       # VPC resolution
│   │   └── loadbalancer/
│   │       └── loadbalancer.go              # LB integration
│   ├── utils/
│   │   └── utils.go                         # Shared utilities
│   └── test/
│       └── pkg/
│           └── environment/
│               └── do/
│                   └── environment.go       # Test environment setup
├── charts/
│   └── karpenter-do/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│           ├── deployment.yaml
│           ├── serviceaccount.yaml
│           ├── clusterrole.yaml
│           ├── clusterrolebinding.yaml
│           └── crds/
│               └── donodeclass.yaml
├── test/
│   └── suites/
│       ├── integration/
│       ├── e2e/
│       ├── disruption/
│       ├── consolidation/
│       ├── drift/
│       ├── scale/
│       └── chaos/
├── hack/
│   └── toolchain.sh
├── designs/
│   └── implementation-plan.md               # This file
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── .golangci.yaml
```
