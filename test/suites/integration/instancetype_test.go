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

package integration

import (
	"testing"

	v1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instancetype"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/pricing"
)

func TestInstanceTypeProvider_List(t *testing.T) {
	pricingProvider := pricing.NewDefaultProvider(env.client)
	if err := pricingProvider.LivePricing(env.ctx); err != nil {
		t.Fatalf("LivePricing() error: %v", err)
	}

	provider := instancetype.NewDefaultProvider(env.client, pricingProvider)

	nodeClass := &v1alpha1.DONodeClass{}
	nodeClass.Spec.Region = env.region

	types, err := provider.List(env.ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(types) == 0 {
		t.Fatal("List() returned no instance types for region", env.region)
	}

	t.Logf("Found %d instance types in region %s", len(types), env.region)

	// Verify structure of a few instance types.
	for i, it := range types {
		if i >= 5 {
			break // just spot-check the first 5
		}

		if it.Name == "" {
			t.Errorf("instance type [%d] has empty name", i)
		}

		cpuQty := it.Capacity[v1.ResourceCPU]
		memQty := it.Capacity[v1.ResourceMemory]
		if cpuQty.IsZero() {
			t.Errorf("instance type %q has zero CPU capacity", it.Name)
		}
		if memQty.IsZero() {
			t.Errorf("instance type %q has zero memory capacity", it.Name)
		}

		if len(it.Offerings) == 0 {
			t.Errorf("instance type %q has no offerings", it.Name)
		}

		t.Logf("  %s: %s vCPU, %s memory, %d offerings",
			it.Name,
			cpuQty.String(),
			memQty.String(),
			len(it.Offerings))
	}
}

func TestInstanceTypeProvider_ListFiltersByRegion(t *testing.T) {
	pricingProvider := pricing.NewDefaultProvider(env.client)
	if err := pricingProvider.LivePricing(env.ctx); err != nil {
		t.Fatalf("LivePricing() error: %v", err)
	}

	provider := instancetype.NewDefaultProvider(env.client, pricingProvider)

	nodeClassRegion := &v1alpha1.DONodeClass{}
	nodeClassRegion.Spec.Region = env.region

	regionTypes, err := provider.List(env.ctx, nodeClassRegion)
	if err != nil {
		t.Fatalf("List(region=%s) error: %v", env.region, err)
	}

	// A made-up region should return zero results (after the cache is populated).
	nodeClassFake := &v1alpha1.DONodeClass{}
	nodeClassFake.Spec.Region = "nonexistent-region"

	fakeTypes, err := provider.List(env.ctx, nodeClassFake)
	if err != nil {
		t.Fatalf("List(region=nonexistent) error: %v", err)
	}

	if len(fakeTypes) != 0 {
		t.Errorf("expected 0 instance types for nonexistent region, got %d", len(fakeTypes))
	}

	t.Logf("region=%s: %d types, region=nonexistent: %d types", env.region, len(regionTypes), len(fakeTypes))
}
