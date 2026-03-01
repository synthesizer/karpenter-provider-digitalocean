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

package instancetype

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/digitalocean/godo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/pricing"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

// Provider returns the available instance types for a given DONodeClass.
type Provider interface {
	List(ctx context.Context, nodeClass *v1alpha1.DONodeClass) ([]*cloudprovider.InstanceType, error)
}

// DefaultProvider implements the instance type Provider with caching.
type DefaultProvider struct {
	doClient        *godo.Client
	pricingProvider pricing.Provider

	mu    sync.RWMutex
	cache []*cloudprovider.InstanceType
}

// NewDefaultProvider creates a new instance type provider.
func NewDefaultProvider(doClient *godo.Client, pricingProvider pricing.Provider) *DefaultProvider {
	return &DefaultProvider{
		doClient:        doClient,
		pricingProvider: pricingProvider,
	}
}

// List returns all DigitalOcean instance types available for the given NodeClass.
func (p *DefaultProvider) List(ctx context.Context, nodeClass *v1alpha1.DONodeClass) ([]*cloudprovider.InstanceType, error) {
	p.mu.RLock()
	if p.cache != nil {
		defer p.mu.RUnlock()
		return p.filterByRegion(p.cache, nodeClass.Spec.Region), nil
	}
	p.mu.RUnlock()

	// Fetch sizes from DigitalOcean API
	instanceTypes, err := p.fetchInstanceTypes(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.cache = instanceTypes
	p.mu.Unlock()

	return p.filterByRegion(instanceTypes, nodeClass.Spec.Region), nil
}

// fetchInstanceTypes retrieves all DigitalOcean sizes and converts them to Karpenter InstanceTypes.
func (p *DefaultProvider) fetchInstanceTypes(ctx context.Context) ([]*cloudprovider.InstanceType, error) {
	sizes, _, err := p.doClient.Sizes.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return nil, fmt.Errorf("listing DO sizes: %w", err)
	}

	var instanceTypes []*cloudprovider.InstanceType
	for _, size := range sizes {
		if !size.Available {
			continue
		}
		it := p.sizeToInstanceType(size)
		if it != nil {
			instanceTypes = append(instanceTypes, it)
		}
	}
	return instanceTypes, nil
}

// sizeToInstanceType converts a DigitalOcean Size to a Karpenter InstanceType.
func (p *DefaultProvider) sizeToInstanceType(size godo.Size) *cloudprovider.InstanceType {
	family := extractFamily(size.Slug)

	// Build requirements
	requirements := scheduling.NewRequirements(
		scheduling.NewRequirement(v1.LabelArchStable, v1.NodeSelectorOpIn, "amd64"),
		scheduling.NewRequirement(v1.LabelOSStable, v1.NodeSelectorOpIn, "linux"),
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, v1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
		scheduling.NewRequirement(v1.LabelInstanceTypeStable, v1.NodeSelectorOpIn, size.Slug),
		scheduling.NewRequirement(v1alpha1.LabelInstanceTypeFamily, v1.NodeSelectorOpIn, family),
	)

	// Add region requirements
	regionReqs := make([]string, len(size.Regions))
	copy(regionReqs, size.Regions)
	requirements.Add(scheduling.NewRequirement(v1.LabelTopologyRegion, v1.NodeSelectorOpIn, regionReqs...))

	// Build offerings (one per region the size is available in)
	var offerings cloudprovider.Offerings
	for _, region := range size.Regions {
		price, ok := p.pricingProvider.InstanceTypePrice(size.Slug, region)
		if !ok {
			price = float64(size.PriceHourly)
		}
		offerings = append(offerings, cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.LabelTopologyRegion, v1.NodeSelectorOpIn, region),
				scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, v1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
			),
			Price:     price,
			Available: true,
		})
	}

	// Calculate capacity (total resources minus system overhead)
	cpuQuantity := resource.MustParse(fmt.Sprintf("%d", size.Vcpus))
	memoryBytes := int64(size.Memory) * 1024 * 1024 // DO returns memory in MB
	memoryQuantity := *resource.NewQuantity(memoryBytes, resource.BinarySI)
	diskBytes := int64(size.Disk) * 1024 * 1024 * 1024 // DO returns disk in GB
	diskQuantity := *resource.NewQuantity(diskBytes, resource.BinarySI)

	// System overhead for kubelet, OS, etc.
	overhead := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("100m"),
		v1.ResourceMemory: resource.MustParse("256Mi"),
	}

	return &cloudprovider.InstanceType{
		Name:         size.Slug,
		Requirements: requirements,
		Offerings:    offerings,
		Capacity: v1.ResourceList{
			v1.ResourceCPU:              cpuQuantity,
			v1.ResourceMemory:           memoryQuantity,
			v1.ResourceEphemeralStorage: diskQuantity,
			v1.ResourcePods:             resource.MustParse("110"), // Default max pods
		},
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved: overhead,
		},
	}
}

// filterByRegion filters instance types to only those available in the specified region.
func (p *DefaultProvider) filterByRegion(instanceTypes []*cloudprovider.InstanceType, region string) []*cloudprovider.InstanceType {
	var filtered []*cloudprovider.InstanceType
	for _, it := range instanceTypes {
		// Check if any offering is in the target region
		for _, offering := range it.Offerings {
			if offering.Requirements.Get(v1.LabelTopologyRegion).Has(region) {
				filtered = append(filtered, it)
				break
			}
		}
	}
	return filtered
}

// extractFamily determines the instance family from a DO size slug.
// Examples:
//
//	"s-1vcpu-2gb"    → "s"   (basic)
//	"g-2vcpu-8gb"    → "g"   (general purpose)
//	"c-2vcpu-4gb"    → "c"   (CPU-optimized)
//	"m-2vcpu-16gb"   → "m"   (memory-optimized)
//	"so-2vcpu-16gb"  → "so"  (storage-optimized)
//	"gd-2vcpu-8gb"   → "gd"  (general purpose + NVMe)
//	"gpu-h100x1-80gb"→ "gpu" (GPU)
func extractFamily(slug string) string {
	parts := strings.SplitN(slug, "-", 2)
	if len(parts) == 0 {
		return "unknown"
	}

	prefix := parts[0]
	// Handle multi-character families
	switch {
	case strings.HasPrefix(slug, "gpu-"):
		return "gpu"
	case strings.HasPrefix(slug, "so-"):
		return "so"
	case strings.HasPrefix(slug, "gd-"):
		return "gd"
	default:
		return prefix
	}
}
