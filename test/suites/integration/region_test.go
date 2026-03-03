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

	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/region"
)

func TestRegionProvider_List(t *testing.T) {
	provider := region.NewDefaultProvider(env.client)

	regions, err := provider.List(env.ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(regions) == 0 {
		t.Fatal("List() returned no regions")
	}

	t.Logf("Found %d available regions: %v", len(regions), regions)

	// The configured test region should be in the list.
	found := false
	for _, r := range regions {
		if r == env.region {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("configured region %q not found in available regions", env.region)
	}
}

func TestRegionProvider_IsAvailable(t *testing.T) {
	provider := region.NewDefaultProvider(env.client)

	if !provider.IsAvailable(env.ctx, env.region) {
		t.Errorf("IsAvailable(%q) = false, want true", env.region)
	}

	if provider.IsAvailable(env.ctx, "nonexistent-region-xyz") {
		t.Error("IsAvailable(\"nonexistent-region-xyz\") = true, want false")
	}
}

func TestRegionProvider_ListCachesResults(t *testing.T) {
	provider := region.NewDefaultProvider(env.client)

	// First call populates cache.
	first, err := provider.List(env.ctx)
	if err != nil {
		t.Fatalf("first List() error: %v", err)
	}

	// Second call should return from cache (same results).
	second, err := provider.List(env.ctx)
	if err != nil {
		t.Fatalf("second List() error: %v", err)
	}

	if len(first) != len(second) {
		t.Errorf("cached result length %d != first result length %d", len(second), len(first))
	}
}
