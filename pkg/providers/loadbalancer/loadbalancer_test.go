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
	"testing"

	fakeproviders "github.com/digitalocean/karpenter-provider-digitalocean/pkg/fake"
)

func TestAddDroplets(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()

	err := provider.AddDroplets(ctx, "lb-1", 100, 200, 300)
	if err != nil {
		t.Fatalf("AddDroplets() unexpected error: %v", err)
	}

	if len(provider.LBDroplets["lb-1"]) != 3 {
		t.Errorf("expected 3 droplets, got %d", len(provider.LBDroplets["lb-1"]))
	}
	if provider.AddCalls != 1 {
		t.Errorf("expected 1 add call, got %d", provider.AddCalls)
	}
}

func TestAddDropletsMultipleCalls(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()

	_ = provider.AddDroplets(ctx, "lb-1", 100)
	_ = provider.AddDroplets(ctx, "lb-1", 200)

	if len(provider.LBDroplets["lb-1"]) != 2 {
		t.Errorf("expected 2 droplets, got %d", len(provider.LBDroplets["lb-1"]))
	}
	if provider.AddCalls != 2 {
		t.Errorf("expected 2 add calls, got %d", provider.AddCalls)
	}
}

func TestAddDropletsError(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()
	provider.AddError = fmt.Errorf("LB not found")

	err := provider.AddDroplets(ctx, "lb-1", 100)
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestRemoveDroplets(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()
	provider.LBDroplets["lb-1"] = []int{100, 200, 300}

	err := provider.RemoveDroplets(ctx, "lb-1", 200)
	if err != nil {
		t.Fatalf("RemoveDroplets() unexpected error: %v", err)
	}

	if len(provider.LBDroplets["lb-1"]) != 2 {
		t.Errorf("expected 2 droplets after remove, got %d", len(provider.LBDroplets["lb-1"]))
	}

	// Verify 200 is actually removed
	for _, id := range provider.LBDroplets["lb-1"] {
		if id == 200 {
			t.Error("droplet 200 should have been removed")
		}
	}
	if provider.RemoveCalls != 1 {
		t.Errorf("expected 1 remove call, got %d", provider.RemoveCalls)
	}
}

func TestRemoveDropletsMultiple(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()
	provider.LBDroplets["lb-1"] = []int{100, 200, 300, 400}

	err := provider.RemoveDroplets(ctx, "lb-1", 200, 400)
	if err != nil {
		t.Fatalf("RemoveDroplets() unexpected error: %v", err)
	}

	if len(provider.LBDroplets["lb-1"]) != 2 {
		t.Errorf("expected 2 droplets after remove, got %d", len(provider.LBDroplets["lb-1"]))
	}
}

func TestRemoveDropletsError(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()
	provider.RemoveError = fmt.Errorf("network error")

	err := provider.RemoveDroplets(ctx, "lb-1", 100)
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestRemoveDropletsNonExistent(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()
	provider.LBDroplets["lb-1"] = []int{100, 200}

	// Remove a non-existent droplet — should be no-op
	err := provider.RemoveDroplets(ctx, "lb-1", 999)
	if err != nil {
		t.Fatalf("RemoveDroplets() unexpected error: %v", err)
	}

	if len(provider.LBDroplets["lb-1"]) != 2 {
		t.Errorf("expected 2 droplets (unchanged), got %d", len(provider.LBDroplets["lb-1"]))
	}
}

func TestMultipleLoadBalancers(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()

	_ = provider.AddDroplets(ctx, "lb-1", 100, 200)
	_ = provider.AddDroplets(ctx, "lb-2", 300, 400)

	if len(provider.LBDroplets["lb-1"]) != 2 {
		t.Errorf("lb-1: expected 2 droplets, got %d", len(provider.LBDroplets["lb-1"]))
	}
	if len(provider.LBDroplets["lb-2"]) != 2 {
		t.Errorf("lb-2: expected 2 droplets, got %d", len(provider.LBDroplets["lb-2"]))
	}
}

func TestAddThenRemoveRoundtrip(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewLoadBalancerProvider()

	// Add
	_ = provider.AddDroplets(ctx, "lb-1", 100, 200, 300)

	// Remove one
	_ = provider.RemoveDroplets(ctx, "lb-1", 200)

	// Verify
	expected := []int{100, 300}
	actual := provider.LBDroplets["lb-1"]
	if len(actual) != len(expected) {
		t.Fatalf("expected %d droplets, got %d", len(expected), len(actual))
	}
	for i, id := range expected {
		if actual[i] != id {
			t.Errorf("droplet[%d] = %d, want %d", i, actual[i], id)
		}
	}
}
