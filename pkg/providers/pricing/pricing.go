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

package pricing

import (
	"context"
	"fmt"
	"sync"

	"github.com/digitalocean/godo"
)

// Provider provides pricing information for DigitalOcean Droplet sizes.
type Provider interface {
	// InstanceTypePrice returns the hourly price for a size in a region.
	// The second return value indicates whether pricing data is available.
	InstanceTypePrice(sizeName, region string) (float64, bool)

	// LivePricing refreshes pricing data from the DigitalOcean API.
	LivePricing(ctx context.Context) error
}

// priceKey is a composite key for the pricing cache.
type priceKey struct {
	size   string
	region string
}

// DefaultProvider implements the pricing Provider with cached pricing data.
type DefaultProvider struct {
	doClient *godo.Client

	mu     sync.RWMutex
	prices map[priceKey]float64
}

// NewDefaultProvider creates a new pricing provider.
func NewDefaultProvider(doClient *godo.Client) *DefaultProvider {
	return &DefaultProvider{
		doClient: doClient,
		prices:   make(map[priceKey]float64),
	}
}

// InstanceTypePrice returns the hourly price for a size in a region.
func (p *DefaultProvider) InstanceTypePrice(sizeName, region string) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	price, ok := p.prices[priceKey{size: sizeName, region: region}]
	return price, ok
}

// LivePricing refreshes pricing data from the DigitalOcean API.
// DigitalOcean pricing is uniform across regions, but we cache per-region
// for consistency with other Karpenter providers.
func (p *DefaultProvider) LivePricing(ctx context.Context) error {
	sizes, _, err := p.doClient.Sizes.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return fmt.Errorf("listing sizes for pricing: %w", err)
	}

	prices := make(map[priceKey]float64, len(sizes)*5) // estimate ~5 regions per size
	for _, size := range sizes {
		for _, region := range size.Regions {
			prices[priceKey{size: size.Slug, region: region}] = size.PriceHourly
		}
	}

	p.mu.Lock()
	p.prices = prices
	p.mu.Unlock()

	return nil
}
