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

package vpc

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
)

// Provider provides VPC information for DigitalOcean.
type Provider interface {
	// Get retrieves a VPC by its UUID.
	Get(ctx context.Context, id string) (*godo.VPC, error)

	// GetDefault returns the default VPC for a region.
	GetDefault(ctx context.Context, region string) (*godo.VPC, error)
}

// DefaultProvider implements the VPC Provider.
type DefaultProvider struct {
	doClient *godo.Client
}

// NewDefaultProvider creates a new VPC provider.
func NewDefaultProvider(doClient *godo.Client) *DefaultProvider {
	return &DefaultProvider{
		doClient: doClient,
	}
}

// Get retrieves a VPC by its UUID.
func (p *DefaultProvider) Get(ctx context.Context, id string) (*godo.VPC, error) {
	vpc, _, err := p.doClient.VPCs.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting VPC %q: %w", id, err)
	}
	return vpc, nil
}

// GetDefault returns the default VPC for a region.
func (p *DefaultProvider) GetDefault(ctx context.Context, region string) (*godo.VPC, error) {
	vpcs, _, err := p.doClient.VPCs.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return nil, fmt.Errorf("listing VPCs: %w", err)
	}
	for _, vpc := range vpcs {
		if vpc.RegionSlug == region && vpc.Default {
			return vpc, nil
		}
	}
	return nil, fmt.Errorf("no default VPC found for region %q", region)
}
