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

package region

import (
	"context"
	"fmt"
	"sync"

	"github.com/digitalocean/godo"
)

// Provider provides region information for DigitalOcean.
type Provider interface {
	// List returns all available regions.
	List(ctx context.Context) ([]string, error)

	// IsAvailable checks if a region is available.
	IsAvailable(ctx context.Context, region string) bool
}

// DefaultProvider implements the region Provider.
type DefaultProvider struct {
	doClient *godo.Client

	mu      sync.RWMutex
	regions map[string]bool
}

// NewDefaultProvider creates a new region provider.
func NewDefaultProvider(doClient *godo.Client) *DefaultProvider {
	return &DefaultProvider{
		doClient: doClient,
		regions:  make(map[string]bool),
	}
}

// List returns all available DigitalOcean regions.
func (p *DefaultProvider) List(ctx context.Context) ([]string, error) {
	p.mu.RLock()
	if len(p.regions) > 0 {
		defer p.mu.RUnlock()
		var result []string
		for r, available := range p.regions {
			if available {
				result = append(result, r)
			}
		}
		return result, nil
	}
	p.mu.RUnlock()

	regions, _, err := p.doClient.Regions.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return nil, fmt.Errorf("listing regions: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var result []string
	for _, r := range regions {
		p.regions[r.Slug] = r.Available
		if r.Available {
			result = append(result, r.Slug)
		}
	}
	return result, nil
}

// IsAvailable checks if a region is available.
func (p *DefaultProvider) IsAvailable(ctx context.Context, region string) bool {
	p.mu.RLock()
	available, ok := p.regions[region]
	p.mu.RUnlock()

	if ok {
		return available
	}

	// Refresh cache
	if _, err := p.List(ctx); err != nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.regions[region]
}
