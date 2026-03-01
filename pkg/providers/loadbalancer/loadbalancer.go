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

package loadbalancer

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
)

// Provider manages load balancer registration for DigitalOcean Droplets.
type Provider interface {
	// AddDroplets registers droplets with a load balancer.
	AddDroplets(ctx context.Context, lbID string, dropletIDs ...int) error

	// RemoveDroplets removes droplets from a load balancer.
	RemoveDroplets(ctx context.Context, lbID string, dropletIDs ...int) error
}

// DefaultProvider implements the load balancer Provider.
type DefaultProvider struct {
	doClient *godo.Client
}

// NewDefaultProvider creates a new load balancer provider.
func NewDefaultProvider(doClient *godo.Client) *DefaultProvider {
	return &DefaultProvider{
		doClient: doClient,
	}
}

// AddDroplets registers droplets with a load balancer.
func (p *DefaultProvider) AddDroplets(ctx context.Context, lbID string, dropletIDs ...int) error {
	_, err := p.doClient.LoadBalancers.AddDroplets(ctx, lbID, dropletIDs...)
	if err != nil {
		return fmt.Errorf("adding droplets to LB %q: %w", lbID, err)
	}
	return nil
}

// RemoveDroplets removes droplets from a load balancer.
func (p *DefaultProvider) RemoveDroplets(ctx context.Context, lbID string, dropletIDs ...int) error {
	_, err := p.doClient.LoadBalancers.RemoveDroplets(ctx, lbID, dropletIDs...)
	if err != nil {
		return fmt.Errorf("removing droplets from LB %q: %w", lbID, err)
	}
	return nil
}
