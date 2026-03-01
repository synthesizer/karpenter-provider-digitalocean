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

package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/digitalocean/godo"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instance"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// --- Instance Provider Mock ---

// InstanceProvider is a fake implementation of the instance.Provider interface.
type InstanceProvider struct {
	mu        sync.Mutex
	Instances map[int]*instance.Instance
	NextID    int

	// Error injection
	CreateError error
	DeleteError error
	GetError    error
	ListError   error

	// Call tracking
	CreateCalls int
	DeleteCalls int
	GetCalls    int
	ListCalls   int
}

// NewInstanceProvider creates a new fake instance provider.
func NewInstanceProvider() *InstanceProvider {
	return &InstanceProvider{
		Instances: make(map[int]*instance.Instance),
		NextID:    100000,
	}
}

func (p *InstanceProvider) Create(_ context.Context, nodeClass *v1alpha1.DONodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*instance.Instance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.CreateCalls++

	if p.CreateError != nil {
		return nil, p.CreateError
	}
	if len(instanceTypes) == 0 {
		return nil, fmt.Errorf("no instance types provided")
	}

	id := p.NextID
	p.NextID++

	inst := &instance.Instance{
		ID:          id,
		Name:        nodeClaim.Name,
		Region:      nodeClass.Spec.Region,
		Size:        instanceTypes[0].Name,
		Status:      "active",
		PrivateIPv4: "10.0.0.2",
		Tags:        []string{v1alpha1.TagManagedBy, v1alpha1.TagClusterPrefix + "test-cluster"},
		ImageID:     nodeClass.Status.ImageID,
		VPCUUID:     nodeClass.Spec.VPCUUID,
	}
	p.Instances[id] = inst
	return inst, nil
}

func (p *InstanceProvider) Delete(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.DeleteCalls++

	if p.DeleteError != nil {
		return p.DeleteError
	}

	var dropletID int
	if _, err := fmt.Sscanf(id, "%d", &dropletID); err != nil {
		return fmt.Errorf("invalid droplet ID %q", id)
	}

	if _, ok := p.Instances[dropletID]; !ok {
		return fmt.Errorf("droplet %d not found", dropletID)
	}
	delete(p.Instances, dropletID)
	return nil
}

func (p *InstanceProvider) Get(_ context.Context, id string) (*instance.Instance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.GetCalls++

	if p.GetError != nil {
		return nil, p.GetError
	}

	var dropletID int
	if _, err := fmt.Sscanf(id, "%d", &dropletID); err != nil {
		return nil, fmt.Errorf("invalid droplet ID %q", id)
	}

	inst, ok := p.Instances[dropletID]
	if !ok {
		return nil, fmt.Errorf("droplet %d not found", dropletID)
	}
	return inst, nil
}

func (p *InstanceProvider) List(_ context.Context) ([]*instance.Instance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ListCalls++

	if p.ListError != nil {
		return nil, p.ListError
	}

	var result []*instance.Instance
	for _, inst := range p.Instances {
		result = append(result, inst)
	}
	return result, nil
}

// --- Instance Type Provider Mock ---

// InstanceTypeProvider is a fake implementation of the instancetype.Provider interface.
type InstanceTypeProvider struct {
	InstanceTypes []*cloudprovider.InstanceType
	ListError     error
	ListCalls     int
}

// NewInstanceTypeProvider creates a fake instance type provider with default types.
func NewInstanceTypeProvider() *InstanceTypeProvider {
	return &InstanceTypeProvider{}
}

func (p *InstanceTypeProvider) List(_ context.Context, _ *v1alpha1.DONodeClass) ([]*cloudprovider.InstanceType, error) {
	p.ListCalls++
	if p.ListError != nil {
		return nil, p.ListError
	}
	return p.InstanceTypes, nil
}

// --- Image Provider Mock ---

// ImageProvider is a fake implementation of the image.Provider interface.
type ImageProvider struct {
	// ImageIDs maps DONodeClass names to resolved image IDs.
	ImageIDs map[string]int
	// DefaultImageID is returned when no specific mapping exists.
	DefaultImageID int

	ResolveError error
	ResolveCalls int
}

// NewImageProvider creates a new fake image provider.
func NewImageProvider() *ImageProvider {
	return &ImageProvider{
		ImageIDs:       make(map[string]int),
		DefaultImageID: 12345678,
	}
}

func (p *ImageProvider) Resolve(_ context.Context, nodeClass *v1alpha1.DONodeClass) (int, error) {
	p.ResolveCalls++
	if p.ResolveError != nil {
		return 0, p.ResolveError
	}

	if nodeClass.Spec.Image.ID != 0 {
		return nodeClass.Spec.Image.ID, nil
	}

	if id, ok := p.ImageIDs[nodeClass.Spec.Image.Slug]; ok {
		return id, nil
	}

	if nodeClass.Spec.Image.Slug == "" {
		return 0, fmt.Errorf("no image slug or ID specified")
	}

	return p.DefaultImageID, nil
}

// --- Pricing Provider Mock ---

// PricingProvider is a fake implementation of the pricing.Provider interface.
type PricingProvider struct {
	Prices map[string]float64 // key: "size:region"

	LivePricingError error
	LivePricingCalls int
}

// NewPricingProvider creates a new fake pricing provider.
func NewPricingProvider() *PricingProvider {
	return &PricingProvider{
		Prices: make(map[string]float64),
	}
}

func (p *PricingProvider) InstanceTypePrice(sizeName, region string) (float64, bool) {
	key := sizeName + ":" + region
	price, ok := p.Prices[key]
	return price, ok
}

func (p *PricingProvider) LivePricing(_ context.Context) error {
	p.LivePricingCalls++
	return p.LivePricingError
}

// --- Region Provider Mock ---

// RegionProvider is a fake implementation of the region.Provider interface.
type RegionProvider struct {
	Regions   map[string]bool
	ListError error
}

// NewRegionProvider creates a new fake region provider.
func NewRegionProvider() *RegionProvider {
	return &RegionProvider{
		Regions: map[string]bool{
			"nyc1": true,
			"nyc3": true,
			"sfo3": true,
			"ams3": true,
			"sgp1": true,
			"lon1": true,
			"fra1": true,
		},
	}
}

func (p *RegionProvider) List(_ context.Context) ([]string, error) {
	if p.ListError != nil {
		return nil, p.ListError
	}
	var result []string
	for r, available := range p.Regions {
		if available {
			result = append(result, r)
		}
	}
	return result, nil
}

func (p *RegionProvider) IsAvailable(_ context.Context, region string) bool {
	return p.Regions[region]
}

// --- VPC Provider Mock ---

// VPCProvider is a fake implementation of the vpc.Provider interface.
type VPCProvider struct {
	VPCs     map[string]*godo.VPC
	GetError error
}

// NewVPCProvider creates a new fake VPC provider.
func NewVPCProvider() *VPCProvider {
	return &VPCProvider{
		VPCs: map[string]*godo.VPC{
			"vpc-123": {
				ID:          "vpc-123",
				URN:         "do:vpc:vpc-123",
				Name:        "test-vpc",
				RegionSlug:  "nyc1",
				Description: "Test VPC",
				IPRange:     "10.10.10.0/24",
				Default:     true,
			},
		},
	}
}

func (p *VPCProvider) Get(_ context.Context, id string) (*godo.VPC, error) {
	if p.GetError != nil {
		return nil, p.GetError
	}
	vpc, ok := p.VPCs[id]
	if !ok {
		return nil, fmt.Errorf("VPC %q not found", id)
	}
	return vpc, nil
}

func (p *VPCProvider) GetDefault(_ context.Context, region string) (*godo.VPC, error) {
	for _, vpc := range p.VPCs {
		if vpc.RegionSlug == region && vpc.Default {
			return vpc, nil
		}
	}
	return nil, fmt.Errorf("no default VPC found for region %q", region)
}

// --- Load Balancer Provider Mock ---

// LoadBalancerProvider is a fake implementation of the loadbalancer.Provider interface.
type LoadBalancerProvider struct {
	// LBDroplets tracks droplet IDs registered with each load balancer.
	LBDroplets map[string][]int

	AddError    error
	RemoveError error
	AddCalls    int
	RemoveCalls int
}

// NewLoadBalancerProvider creates a new fake load balancer provider.
func NewLoadBalancerProvider() *LoadBalancerProvider {
	return &LoadBalancerProvider{
		LBDroplets: make(map[string][]int),
	}
}

func (p *LoadBalancerProvider) AddDroplets(_ context.Context, lbID string, dropletIDs ...int) error {
	p.AddCalls++
	if p.AddError != nil {
		return p.AddError
	}
	p.LBDroplets[lbID] = append(p.LBDroplets[lbID], dropletIDs...)
	return nil
}

func (p *LoadBalancerProvider) RemoveDroplets(_ context.Context, lbID string, dropletIDs ...int) error {
	p.RemoveCalls++
	if p.RemoveError != nil {
		return p.RemoveError
	}
	existing := p.LBDroplets[lbID]
	removeSet := make(map[int]bool)
	for _, id := range dropletIDs {
		removeSet[id] = true
	}
	var remaining []int
	for _, id := range existing {
		if !removeSet[id] {
			remaining = append(remaining, id)
		}
	}
	p.LBDroplets[lbID] = remaining
	return nil
}
