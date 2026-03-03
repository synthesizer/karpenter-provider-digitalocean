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
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/godo"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// Provider manages the lifecycle of DOKS node pools for Karpenter.
// Each Karpenter NodeClaim maps to a single DOKS node pool with Count=1,
// allowing individual control over node size and clean deletion semantics.
type Provider interface {
	// Create creates a new DOKS node pool with Count=1 based on the NodeClaim
	// and DONodeClass specs. It waits for the node to be provisioned and returns
	// the instance details including the underlying droplet ID.
	Create(ctx context.Context, nodeClass *v1alpha1.DONodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*Instance, error)

	// Delete removes a DOKS node pool by its node pool ID.
	Delete(ctx context.Context, nodePoolID string) error

	// Get retrieves an instance by its droplet ID by searching all Karpenter-managed
	// DOKS node pools in the cluster.
	Get(ctx context.Context, dropletID string) (*Instance, error)

	// List returns all Karpenter-managed instances from DOKS node pools.
	List(ctx context.Context) ([]*Instance, error)
}

// DefaultProvider implements the instance Provider using the DOKS Node Pool API.
type DefaultProvider struct {
	doClient    *godo.Client
	clusterID   string
	clusterName string
	region      string
}

// NewDefaultProvider creates a new instance provider backed by the DOKS Node Pool API.
func NewDefaultProvider(doClient *godo.Client, clusterID, clusterName, region string) *DefaultProvider {
	return &DefaultProvider{
		doClient:    doClient,
		clusterID:   clusterID,
		clusterName: clusterName,
		region:      region,
	}
}

// Create creates a new DOKS node pool with Count=1 for the given NodeClaim.
// It polls until the node pool has a provisioned node with a droplet ID,
// then returns the instance details.
func (p *DefaultProvider) Create(ctx context.Context, nodeClass *v1alpha1.DONodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*Instance, error) {
	if len(instanceTypes) == 0 {
		return nil, fmt.Errorf("no instance types provided")
	}

	// Select the first (cheapest/best-fit) instance type
	// TODO: Implement proper instance type selection (cheapest that fits)
	selectedType := instanceTypes[0]

	// Build tags for the node pool
	tags := []string{
		v1alpha1.TagManagedBy,
		v1alpha1.TagClusterPrefix + p.clusterName,
	}
	tags = append(tags, nodeClass.Spec.Tags...)

	// Build labels for the node pool (propagated to Kubernetes nodes)
	labels := map[string]string{
		v1alpha1.LabelInstanceSize: selectedType.Name,
		v1alpha1.LabelRegion:       p.region,
	}

	// Generate a unique node pool name
	// DOKS node pool names must be <= 255 chars, alphanumeric + hyphens
	nodePoolName := fmt.Sprintf("karp-%s", nodeClaim.Name)
	if len(nodePoolName) > 255 {
		nodePoolName = nodePoolName[:255]
	}

	// Create the DOKS node pool with Count=1
	createReq := &godo.KubernetesNodePoolCreateRequest{
		Name:   nodePoolName,
		Size:   selectedType.Name,
		Count:  1,
		Tags:   tags,
		Labels: labels,
	}

	nodePool, _, err := p.doClient.Kubernetes.CreateNodePool(ctx, p.clusterID, createReq)
	if err != nil {
		return nil, fmt.Errorf("creating DOKS node pool: %w", err)
	}

	// Poll until the node is provisioned and has a droplet ID
	inst, err := p.waitForNode(ctx, nodePool.ID)
	if err != nil {
		// Clean up the node pool if we fail to get the node info
		_, _ = p.doClient.Kubernetes.DeleteNodePool(ctx, p.clusterID, nodePool.ID)
		return nil, fmt.Errorf("waiting for node provisioning: %w", err)
	}

	return inst, nil
}

// waitForNode polls the DOKS API until the node pool has a provisioned node
// with a droplet ID assigned. Returns the instance details.
func (p *DefaultProvider) waitForNode(ctx context.Context, nodePoolID string) (*Instance, error) {
	const (
		maxAttempts = 60
		pollInterval = 5 * time.Second
	)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		nodePool, _, err := p.doClient.Kubernetes.GetNodePool(ctx, p.clusterID, nodePoolID)
		if err != nil {
			return nil, fmt.Errorf("getting node pool %s: %w", nodePoolID, err)
		}

		// Check if any node has a droplet ID assigned
		for _, node := range nodePool.Nodes {
			if node.DropletID != "" && node.DropletID != "0" {
				return nodePoolNodeToInstance(nodePool, node, p.region), nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while waiting for node provisioning: %w", ctx.Err())
		case <-time.After(pollInterval):
			continue
		}
	}

	return nil, fmt.Errorf("timed out waiting for node in pool %s to be provisioned", nodePoolID)
}

// Delete removes a DOKS node pool by its ID.
func (p *DefaultProvider) Delete(ctx context.Context, nodePoolID string) error {
	_, err := p.doClient.Kubernetes.DeleteNodePool(ctx, p.clusterID, nodePoolID)
	if err != nil {
		return fmt.Errorf("deleting DOKS node pool %s: %w", nodePoolID, err)
	}
	return nil
}

// Get retrieves an instance by its droplet ID. It searches all Karpenter-managed
// DOKS node pools in the cluster for a node with the matching droplet ID.
func (p *DefaultProvider) Get(ctx context.Context, dropletID string) (*Instance, error) {
	nodePools, err := p.listManagedNodePools(ctx)
	if err != nil {
		return nil, err
	}

	for _, np := range nodePools {
		for _, node := range np.Nodes {
			if node.DropletID == dropletID {
				return nodePoolNodeToInstance(np, node, p.region), nil
			}
		}
	}

	return nil, fmt.Errorf("no DOKS node found with droplet ID %s", dropletID)
}

// List returns all Karpenter-managed instances from DOKS node pools.
func (p *DefaultProvider) List(ctx context.Context) ([]*Instance, error) {
	nodePools, err := p.listManagedNodePools(ctx)
	if err != nil {
		return nil, err
	}

	var instances []*Instance
	for _, np := range nodePools {
		for _, node := range np.Nodes {
			if node.DropletID != "" && node.DropletID != "0" {
				instances = append(instances, nodePoolNodeToInstance(np, node, p.region))
			}
		}
	}
	return instances, nil
}

// listManagedNodePools lists all DOKS node pools in the cluster that are
// tagged as Karpenter-managed.
func (p *DefaultProvider) listManagedNodePools(ctx context.Context) ([]*godo.KubernetesNodePool, error) {
	allPools, _, err := p.doClient.Kubernetes.ListNodePools(ctx, p.clusterID, &godo.ListOptions{
		PerPage: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("listing DOKS node pools: %w", err)
	}

	// Filter to only Karpenter-managed node pools
	var managed []*godo.KubernetesNodePool
	for _, np := range allPools {
		if hasTag(np.Tags, v1alpha1.TagManagedBy) {
			managed = append(managed, np)
		}
	}
	return managed, nil
}

// nodePoolNodeToInstance converts a DOKS node pool and node to our Instance type.
func nodePoolNodeToInstance(np *godo.KubernetesNodePool, node *godo.KubernetesNode, region string) *Instance {
	inst := &Instance{
		NodePoolID: np.ID,
		DropletID:  node.DropletID,
		Name:       node.Name,
		Region:     region,
		Size:       np.Size,
		Labels:     np.Labels,
		Tags:       np.Tags,
		CreatedAt:  node.CreatedAt,
	}

	// Map DOKS node status
	if node.Status != nil {
		inst.Status = node.Status.State
	}

	return inst
}

// hasTag checks if a tag list contains a specific tag.
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
