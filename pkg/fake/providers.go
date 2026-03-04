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
	"time"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instance"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// --- Instance Provider Mock ---

// InstanceProvider is a fake implementation of the instance.Provider interface.
// It models DOKS node pools — each entry in NodePools represents one node pool
// with a single node (Count=1), keyed by the node pool ID.
type InstanceProvider struct {
	mu sync.Mutex

	// NodePools stores fake instances keyed by NodePoolID.
	NodePools map[string]*instance.Instance

	// NextNodePoolID is a counter used to generate unique node pool IDs.
	NextNodePoolID int

	// NextDropletID is a counter used to generate unique droplet IDs.
	NextDropletID int

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

	// LastInstanceTypes records the instance types passed to the most recent Create call.
	LastInstanceTypes []*cloudprovider.InstanceType
}

// NewInstanceProvider creates a new fake instance provider for DOKS node pools.
func NewInstanceProvider() *InstanceProvider {
	return &InstanceProvider{
		NodePools:      make(map[string]*instance.Instance),
		NextNodePoolID: 1,
		NextDropletID:  100000,
	}
}

func (p *InstanceProvider) Create(_ context.Context, nodeClass *v1alpha1.DONodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*instance.Instance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.CreateCalls++
	p.LastInstanceTypes = instanceTypes

	if p.CreateError != nil {
		return nil, p.CreateError
	}
	if len(instanceTypes) == 0 {
		return nil, fmt.Errorf("no instance types provided")
	}

	nodePoolID := fmt.Sprintf("nodepool-%d", p.NextNodePoolID)
	p.NextNodePoolID++

	dropletID := fmt.Sprintf("%d", p.NextDropletID)
	p.NextDropletID++

	inst := &instance.Instance{
		NodePoolID: nodePoolID,
		DropletID:  dropletID,
		Name:       fmt.Sprintf("karp-%s", nodeClaim.Name),
		Region:     nodeClass.Spec.Region,
		Size:       instanceTypes[0].Name,
		Status:     "running",
		Labels: map[string]string{
			v1alpha1.LabelInstanceSize: instanceTypes[0].Name,
			v1alpha1.LabelRegion:       nodeClass.Spec.Region,
		},
		Tags:      append([]string{v1alpha1.TagManagedBy, v1alpha1.TagClusterPrefix + "test-cluster"}, nodeClass.Spec.Tags...),
		CreatedAt: time.Now(),
	}
	p.NodePools[nodePoolID] = inst
	return inst, nil
}

func (p *InstanceProvider) Delete(_ context.Context, nodePoolID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.DeleteCalls++

	if p.DeleteError != nil {
		return p.DeleteError
	}

	if _, ok := p.NodePools[nodePoolID]; !ok {
		return fmt.Errorf("node pool %q not found", nodePoolID)
	}
	delete(p.NodePools, nodePoolID)
	return nil
}

func (p *InstanceProvider) Get(_ context.Context, dropletID string) (*instance.Instance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.GetCalls++

	if p.GetError != nil {
		return nil, p.GetError
	}

	// Search all node pools for the instance with the matching droplet ID
	for _, inst := range p.NodePools {
		if inst.DropletID == dropletID {
			return inst, nil
		}
	}

	return nil, fmt.Errorf("no instance found with droplet ID %q", dropletID)
}

func (p *InstanceProvider) List(_ context.Context) ([]*instance.Instance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ListCalls++

	if p.ListError != nil {
		return nil, p.ListError
	}

	var result []*instance.Instance
	for _, inst := range p.NodePools {
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
