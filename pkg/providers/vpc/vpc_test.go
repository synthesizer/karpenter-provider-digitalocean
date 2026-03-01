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
	"testing"

	"github.com/digitalocean/godo"

	fakeproviders "github.com/digitalocean/karpenter-provider-digitalocean/pkg/fake"
)

func TestGetVPC(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewVPCProvider()

	vpc, err := provider.Get(ctx, "vpc-123")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if vpc.ID != "vpc-123" {
		t.Errorf("expected VPC ID %q, got %q", "vpc-123", vpc.ID)
	}
	if vpc.Name != "test-vpc" {
		t.Errorf("expected VPC name %q, got %q", "test-vpc", vpc.Name)
	}
	if vpc.RegionSlug != "nyc1" {
		t.Errorf("expected VPC region %q, got %q", "nyc1", vpc.RegionSlug)
	}
	if vpc.IPRange != "10.10.10.0/24" {
		t.Errorf("expected VPC IP range %q, got %q", "10.10.10.0/24", vpc.IPRange)
	}
}

func TestGetVPCNotFound(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewVPCProvider()

	_, err := provider.Get(ctx, "nonexistent-vpc")
	if err == nil {
		t.Fatal("expected error for non-existent VPC")
	}
}

func TestGetVPCError(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewVPCProvider()
	provider.GetError = context.DeadlineExceeded

	_, err := provider.Get(ctx, "vpc-123")
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestGetDefaultVPC(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewVPCProvider()

	vpc, err := provider.GetDefault(ctx, "nyc1")
	if err != nil {
		t.Fatalf("GetDefault() unexpected error: %v", err)
	}
	if vpc.ID != "vpc-123" {
		t.Errorf("expected VPC ID %q, got %q", "vpc-123", vpc.ID)
	}
	if !vpc.Default {
		t.Error("expected VPC to be default")
	}
}

func TestGetDefaultVPCNoDefault(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewVPCProvider()

	// Try to get default for a region with no default VPC
	_, err := provider.GetDefault(ctx, "sfo3")
	if err == nil {
		t.Fatal("expected error when no default VPC exists for region")
	}
}

func TestGetDefaultVPCMultipleRegions(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewVPCProvider()

	// Add another default VPC for a different region
	provider.VPCs["vpc-456"] = &godo.VPC{
		ID:         "vpc-456",
		Name:       "sfo3-vpc",
		RegionSlug: "sfo3",
		Default:    true,
	}

	// Get default for nyc1
	vpc, err := provider.GetDefault(ctx, "nyc1")
	if err != nil {
		t.Fatalf("GetDefault(nyc1) unexpected error: %v", err)
	}
	if vpc.ID != "vpc-123" {
		t.Errorf("expected VPC ID %q for nyc1, got %q", "vpc-123", vpc.ID)
	}

	// Get default for sfo3
	vpc2, err := provider.GetDefault(ctx, "sfo3")
	if err != nil {
		t.Fatalf("GetDefault(sfo3) unexpected error: %v", err)
	}
	if vpc2.ID != "vpc-456" {
		t.Errorf("expected VPC ID %q for sfo3, got %q", "vpc-456", vpc2.ID)
	}
}
