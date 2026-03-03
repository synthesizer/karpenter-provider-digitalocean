/*
Copyright 2024 The Kubernetes Authors.

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

package v1alpha1

const (
	// Group is the API group for the DigitalOcean Karpenter provider.
	Group = "karpenter.do.sh"

	// DONodeClassKind is the Kind for DONodeClass resources.
	DONodeClassKind = "DONodeClass"

	// AnnotationProviderID is the annotation key for the DO provider ID.
	AnnotationProviderID = Group + "/provider-id"

	// AnnotationNodePoolID stores the DOKS node pool UUID on the NodeClaim.
	// This allows efficient deletion without searching all node pools.
	AnnotationNodePoolID = Group + "/node-pool-id"

	// AnnotationDropletID is the annotation key for the DigitalOcean Droplet ID.
	AnnotationDropletID = Group + "/droplet-id"

	// TagManagedBy is the tag applied to all DOKS node pools managed by Karpenter.
	TagManagedBy = "karpenter-managed"

	// TagNodePoolPrefix is the tag prefix for identifying which Karpenter NodePool manages a DOKS node pool.
	TagNodePoolPrefix = "karpenter-nodepool-"

	// TagClusterPrefix is the tag prefix for identifying which cluster a DOKS node pool belongs to.
	TagClusterPrefix = "karpenter-cluster-"

	// LabelInstanceTypeFamily indicates the DigitalOcean instance type family.
	// Families: "s" (basic/shared), "g" (general purpose), "c" (CPU-optimized),
	// "m" (memory-optimized), "so" (storage-optimized), "gd" (general purpose + NVMe),
	// "gpu" (GPU droplets).
	LabelInstanceTypeFamily = Group + "/instance-type-family"

	// LabelInstanceSize is the full DigitalOcean size slug (e.g., "s-1vcpu-2gb").
	LabelInstanceSize = Group + "/instance-size"

	// LabelRegion is the DigitalOcean region slug (e.g., "nyc1", "sfo3").
	LabelRegion = Group + "/region"

	// LabelNodePoolID is the DOKS node pool UUID label on the Kubernetes node.
	LabelNodePoolID = Group + "/node-pool-id"
)

// Instance type families available on DigitalOcean.
const (
	InstanceFamilyBasic            = "s"   // Shared CPU, burstable
	InstanceFamilyGeneralPurpose   = "g"   // Dedicated CPU, balanced
	InstanceFamilyCPUOptimized     = "c"   // Dedicated CPU, compute-heavy
	InstanceFamilyMemoryOptimized  = "m"   // Dedicated CPU, memory-heavy
	InstanceFamilyStorageOptimized = "so"  // Dedicated CPU, NVMe storage
	InstanceFamilyGPU              = "gpu" // GPU-enabled
)
