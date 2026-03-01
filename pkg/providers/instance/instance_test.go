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

package instance

import (
	"testing"

	"github.com/digitalocean/godo"
)

func TestHasTag(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		tag      string
		expected bool
	}{
		{
			name:     "tag found",
			tags:     []string{"karpenter-managed", "karpenter-cluster-test"},
			tag:      "karpenter-managed",
			expected: true,
		},
		{
			name:     "tag not found",
			tags:     []string{"karpenter-managed", "karpenter-cluster-test"},
			tag:      "other-tag",
			expected: false,
		},
		{
			name:     "empty tags",
			tags:     []string{},
			tag:      "karpenter-managed",
			expected: false,
		},
		{
			name:     "nil tags",
			tags:     nil,
			tag:      "karpenter-managed",
			expected: false,
		},
		{
			name:     "empty tag search",
			tags:     []string{"karpenter-managed"},
			tag:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTag(tt.tags, tt.tag)
			if got != tt.expected {
				t.Errorf("hasTag(%v, %q) = %v, want %v", tt.tags, tt.tag, got, tt.expected)
			}
		})
	}
}

func TestDerefString(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected string
	}{
		{
			name:     "nil pointer",
			input:    nil,
			expected: "",
		},
		{
			name:     "non-nil pointer with value",
			input:    strPtr("hello world"),
			expected: "hello world",
		},
		{
			name:     "non-nil pointer with empty string",
			input:    strPtr(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derefString(tt.input)
			if got != tt.expected {
				t.Errorf("derefString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDropletToInstance(t *testing.T) {
	tests := []struct {
		name     string
		droplet  *godo.Droplet
		expected *Instance
	}{
		{
			name: "full droplet conversion",
			droplet: &godo.Droplet{
				ID:   12345,
				Name: "test-node",
				Region: &godo.Region{
					Slug: "nyc1",
					Name: "New York 1",
				},
				Size: &godo.Size{
					Slug: "s-2vcpu-4gb",
				},
				Status:  "active",
				Tags:    []string{"karpenter-managed", "karpenter-cluster-test"},
				VPCUUID: "vpc-abc-123",
				Image: &godo.Image{
					ID:   99999,
					Slug: "ubuntu-24-04-x64",
				},
				Networks: &godo.Networks{
					V4: []godo.NetworkV4{
						{IPAddress: "10.0.0.2", Type: "private"},
						{IPAddress: "1.2.3.4", Type: "public"},
					},
				},
			},
			expected: &Instance{
				ID:          12345,
				Name:        "test-node",
				Region:      "nyc1",
				Size:        "s-2vcpu-4gb",
				Status:      "active",
				PrivateIPv4: "10.0.0.2",
				PublicIPv4:  "1.2.3.4",
				Tags:        []string{"karpenter-managed", "karpenter-cluster-test"},
				ImageID:     99999,
				VPCUUID:     "vpc-abc-123",
			},
		},
		{
			name: "droplet with no networks",
			droplet: &godo.Droplet{
				ID:   67890,
				Name: "no-net-node",
				Region: &godo.Region{
					Slug: "sfo3",
				},
				Size: &godo.Size{
					Slug: "s-1vcpu-1gb",
				},
				Status: "new",
			},
			expected: &Instance{
				ID:     67890,
				Name:   "no-net-node",
				Region: "sfo3",
				Size:   "s-1vcpu-1gb",
				Status: "new",
			},
		},
		{
			name: "droplet with private IP only",
			droplet: &godo.Droplet{
				ID:   11111,
				Name: "private-only",
				Region: &godo.Region{
					Slug: "ams3",
				},
				Size: &godo.Size{
					Slug: "g-2vcpu-8gb",
				},
				Status: "active",
				Networks: &godo.Networks{
					V4: []godo.NetworkV4{
						{IPAddress: "10.10.10.5", Type: "private"},
					},
				},
			},
			expected: &Instance{
				ID:          11111,
				Name:        "private-only",
				Region:      "ams3",
				Size:        "g-2vcpu-8gb",
				Status:      "active",
				PrivateIPv4: "10.10.10.5",
			},
		},
		{
			name: "droplet with no image",
			droplet: &godo.Droplet{
				ID:   22222,
				Name: "no-image",
				Region: &godo.Region{
					Slug: "lon1",
				},
				Size: &godo.Size{
					Slug: "s-1vcpu-1gb",
				},
				Status: "active",
			},
			expected: &Instance{
				ID:     22222,
				Name:   "no-image",
				Region: "lon1",
				Size:   "s-1vcpu-1gb",
				Status: "active",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dropletToInstance(tt.droplet)

			if got.ID != tt.expected.ID {
				t.Errorf("ID = %d, want %d", got.ID, tt.expected.ID)
			}
			if got.Name != tt.expected.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.expected.Name)
			}
			if got.Region != tt.expected.Region {
				t.Errorf("Region = %q, want %q", got.Region, tt.expected.Region)
			}
			if got.Size != tt.expected.Size {
				t.Errorf("Size = %q, want %q", got.Size, tt.expected.Size)
			}
			if got.Status != tt.expected.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.expected.Status)
			}
			if got.PrivateIPv4 != tt.expected.PrivateIPv4 {
				t.Errorf("PrivateIPv4 = %q, want %q", got.PrivateIPv4, tt.expected.PrivateIPv4)
			}
			if got.PublicIPv4 != tt.expected.PublicIPv4 {
				t.Errorf("PublicIPv4 = %q, want %q", got.PublicIPv4, tt.expected.PublicIPv4)
			}
			if got.ImageID != tt.expected.ImageID {
				t.Errorf("ImageID = %d, want %d", got.ImageID, tt.expected.ImageID)
			}
			if got.VPCUUID != tt.expected.VPCUUID {
				t.Errorf("VPCUUID = %q, want %q", got.VPCUUID, tt.expected.VPCUUID)
			}
			if len(got.Tags) != len(tt.expected.Tags) {
				t.Errorf("Tags length = %d, want %d", len(got.Tags), len(tt.expected.Tags))
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
