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
	"time"

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

func TestNodePoolNodeToInstance(t *testing.T) {
	createdAt := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		np       *godo.KubernetesNodePool
		node     *godo.KubernetesNode
		region   string
		expected *Instance
	}{
		{
			name: "full node pool node conversion",
			np: &godo.KubernetesNodePool{
				ID:   "pool-abc-123",
				Name: "karp-test-node",
				Size: "s-2vcpu-4gb",
				Tags: []string{"karpenter-managed", "karpenter-cluster-test"},
				Labels: map[string]string{
					"karpenter.do.sh/instance-size": "s-2vcpu-4gb",
					"karpenter.do.sh/region":        "nyc1",
				},
			},
			node: &godo.KubernetesNode{
				ID:        "node-xyz",
				Name:      "karp-test-node-abc",
				DropletID: "12345",
				Status: &godo.KubernetesNodeStatus{
					State: "running",
				},
				CreatedAt: createdAt,
			},
			region: "nyc1",
			expected: &Instance{
				NodePoolID: "pool-abc-123",
				DropletID:  "12345",
				Name:       "karp-test-node-abc",
				Region:     "nyc1",
				Size:       "s-2vcpu-4gb",
				Status:     "running",
				Tags:       []string{"karpenter-managed", "karpenter-cluster-test"},
				Labels: map[string]string{
					"karpenter.do.sh/instance-size": "s-2vcpu-4gb",
					"karpenter.do.sh/region":        "nyc1",
				},
				CreatedAt: createdAt,
			},
		},
		{
			name: "node with nil status",
			np: &godo.KubernetesNodePool{
				ID:   "pool-def-456",
				Name: "karp-no-status",
				Size: "s-1vcpu-1gb",
			},
			node: &godo.KubernetesNode{
				ID:        "node-no-status",
				Name:      "karp-no-status-node",
				DropletID: "67890",
				Status:    nil,
				CreatedAt: createdAt,
			},
			region: "sfo3",
			expected: &Instance{
				NodePoolID: "pool-def-456",
				DropletID:  "67890",
				Name:       "karp-no-status-node",
				Region:     "sfo3",
				Size:       "s-1vcpu-1gb",
				Status:     "",
				CreatedAt:  createdAt,
			},
		},
		{
			name: "node with empty labels and tags",
			np: &godo.KubernetesNodePool{
				ID:     "pool-ghi-789",
				Name:   "karp-minimal",
				Size:   "g-2vcpu-8gb",
				Tags:   nil,
				Labels: nil,
			},
			node: &godo.KubernetesNode{
				ID:        "node-minimal",
				Name:      "karp-minimal-node",
				DropletID: "11111",
				Status: &godo.KubernetesNodeStatus{
					State: "provisioning",
				},
				CreatedAt: createdAt,
			},
			region: "ams3",
			expected: &Instance{
				NodePoolID: "pool-ghi-789",
				DropletID:  "11111",
				Name:       "karp-minimal-node",
				Region:     "ams3",
				Size:       "g-2vcpu-8gb",
				Status:     "provisioning",
				Tags:       nil,
				Labels:     nil,
				CreatedAt:  createdAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodePoolNodeToInstance(tt.np, tt.node, tt.region)

			if got.NodePoolID != tt.expected.NodePoolID {
				t.Errorf("NodePoolID = %q, want %q", got.NodePoolID, tt.expected.NodePoolID)
			}
			if got.DropletID != tt.expected.DropletID {
				t.Errorf("DropletID = %q, want %q", got.DropletID, tt.expected.DropletID)
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
			if len(got.Tags) != len(tt.expected.Tags) {
				t.Errorf("Tags length = %d, want %d", len(got.Tags), len(tt.expected.Tags))
			}
			if len(got.Labels) != len(tt.expected.Labels) {
				t.Errorf("Labels length = %d, want %d", len(got.Labels), len(tt.expected.Labels))
			}
			if !got.CreatedAt.Equal(tt.expected.CreatedAt) {
				t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, tt.expected.CreatedAt)
			}
		})
	}
}
