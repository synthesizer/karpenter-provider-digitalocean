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

	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/pricing"
)

func TestPricingProvider_LivePricing(t *testing.T) {
	provider := pricing.NewDefaultProvider(env.client)

	// Before refresh, no prices should be available.
	_, ok := provider.InstanceTypePrice("s-1vcpu-1gb", env.region)
	if ok {
		t.Log("price was already cached (unexpected but not fatal)")
	}

	// Refresh pricing from the live API.
	if err := provider.LivePricing(env.ctx); err != nil {
		t.Fatalf("LivePricing() error: %v", err)
	}

	// After refresh, common sizes should have prices.
	commonSizes := []string{
		"s-1vcpu-1gb",
		"s-1vcpu-2gb",
		"s-2vcpu-4gb",
	}

	for _, size := range commonSizes {
		price, ok := provider.InstanceTypePrice(size, env.region)
		if !ok {
			t.Errorf("InstanceTypePrice(%q, %q) not found after LivePricing", size, env.region)
			continue
		}
		if price <= 0 {
			t.Errorf("InstanceTypePrice(%q, %q) = %f, want > 0", size, env.region, price)
		}
		t.Logf("  %s in %s: $%.4f/hr", size, env.region, price)
	}
}

func TestPricingProvider_UnknownSize(t *testing.T) {
	provider := pricing.NewDefaultProvider(env.client)

	if err := provider.LivePricing(env.ctx); err != nil {
		t.Fatalf("LivePricing() error: %v", err)
	}

	_, ok := provider.InstanceTypePrice("nonexistent-size-slug", env.region)
	if ok {
		t.Error("InstanceTypePrice for nonexistent size should return false")
	}
}
