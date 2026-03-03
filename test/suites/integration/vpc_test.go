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

	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/vpc"
)

func TestVPCProvider_GetDefault(t *testing.T) {
	provider := vpc.NewDefaultProvider(env.client)

	defaultVPC, err := provider.GetDefault(env.ctx, env.region)
	if err != nil {
		t.Fatalf("GetDefault(%q) error: %v", env.region, err)
	}

	if defaultVPC.ID == "" {
		t.Error("default VPC has empty ID")
	}
	if defaultVPC.RegionSlug != env.region {
		t.Errorf("default VPC region = %q, want %q", defaultVPC.RegionSlug, env.region)
	}
	if !defaultVPC.Default {
		t.Error("returned VPC is not marked as default")
	}

	t.Logf("Default VPC in %s: %s (%s)", env.region, defaultVPC.Name, defaultVPC.ID)
}

func TestVPCProvider_Get(t *testing.T) {
	provider := vpc.NewDefaultProvider(env.client)

	// If DO_VPC_UUID is set, look it up directly.
	vpcUUID := env.vpcUUID
	if vpcUUID == "" {
		// Fall back to the default VPC.
		defaultVPC, err := provider.GetDefault(env.ctx, env.region)
		if err != nil {
			t.Fatalf("GetDefault(%q) error: %v", env.region, err)
		}
		vpcUUID = defaultVPC.ID
	}

	got, err := provider.Get(env.ctx, vpcUUID)
	if err != nil {
		t.Fatalf("Get(%q) error: %v", vpcUUID, err)
	}

	if got.ID != vpcUUID {
		t.Errorf("Get() returned VPC ID %q, want %q", got.ID, vpcUUID)
	}

	t.Logf("VPC %s: name=%s, region=%s, ip_range=%s", got.ID, got.Name, got.RegionSlug, got.IPRange)
}

func TestVPCProvider_GetNonexistent(t *testing.T) {
	provider := vpc.NewDefaultProvider(env.client)

	_, err := provider.Get(env.ctx, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Error("Get(nonexistent UUID) should return error")
	}
	t.Logf("Expected error for nonexistent VPC: %v", err)
}
