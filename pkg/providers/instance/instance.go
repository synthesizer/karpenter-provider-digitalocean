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
	"strconv"

	"github.com/digitalocean/godo"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// Provider manages the lifecycle of DigitalOcean Droplets for Karpenter.
type Provider interface {
	// Create launches a new Droplet based on the NodeClaim and DONodeClass specs.
	Create(ctx context.Context, nodeClass *v1alpha1.DONodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*Instance, error)

	// Delete terminates a Droplet by its ID.
	Delete(ctx context.Context, id string) error

	// Get retrieves a Droplet by its ID.
	Get(ctx context.Context, id string) (*Instance, error)

	// List returns all Karpenter-managed Droplets.
	List(ctx context.Context) ([]*Instance, error)
}

// DefaultProvider implements the instance Provider using the DigitalOcean API.
type DefaultProvider struct {
	doClient    *godo.Client
	clusterName string
}

// NewDefaultProvider creates a new instance provider.
func NewDefaultProvider(doClient *godo.Client, clusterName string) *DefaultProvider {
	return &DefaultProvider{
		doClient:    doClient,
		clusterName: clusterName,
	}
}

// Create launches a new DigitalOcean Droplet.
func (p *DefaultProvider) Create(ctx context.Context, nodeClass *v1alpha1.DONodeClass, nodeClaim *karpv1.NodeClaim, instanceTypes []*cloudprovider.InstanceType) (*Instance, error) {
	if len(instanceTypes) == 0 {
		return nil, fmt.Errorf("no instance types provided")
	}

	// Select the best instance type based on requirements
	// TODO: Implement proper instance type selection (cheapest that fits)
	selectedType := instanceTypes[0]

	// Build tags
	tags := []string{
		v1alpha1.TagManagedBy,
		v1alpha1.TagClusterPrefix + p.clusterName,
	}
	tags = append(tags, nodeClass.Spec.Tags...)

	// Resolve image
	imageID := nodeClass.Status.ImageID
	if imageID == 0 && nodeClass.Spec.Image.ID != 0 {
		imageID = nodeClass.Spec.Image.ID
	}

	// Build the create request
	createReq := &godo.DropletCreateRequest{
		Name:   nodeClaim.Name,
		Region: nodeClass.Spec.Region,
		Size:   selectedType.Name,
		Image: godo.DropletCreateImage{
			ID: imageID,
		},
		Tags:     tags,
		VPCUUID:  nodeClass.Spec.VPCUUID,
		UserData: derefString(nodeClass.Spec.UserData),
	}

	// Add SSH keys
	for _, key := range nodeClass.Spec.SSHKeys {
		createReq.SSHKeys = append(createReq.SSHKeys, godo.DropletCreateSSHKey{
			Fingerprint: key,
		})
	}

	// Create the droplet
	droplet, _, err := p.doClient.Droplets.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("creating droplet: %w", err)
	}

	return dropletToInstance(droplet), nil
}

// Delete terminates a DigitalOcean Droplet.
func (p *DefaultProvider) Delete(ctx context.Context, id string) error {
	dropletID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("invalid droplet ID %q: %w", id, err)
	}
	_, err = p.doClient.Droplets.Delete(ctx, dropletID)
	if err != nil {
		return fmt.Errorf("deleting droplet %d: %w", dropletID, err)
	}
	return nil
}

// Get retrieves a DigitalOcean Droplet by ID.
func (p *DefaultProvider) Get(ctx context.Context, id string) (*Instance, error) {
	dropletID, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid droplet ID %q: %w", id, err)
	}
	droplet, _, err := p.doClient.Droplets.Get(ctx, dropletID)
	if err != nil {
		return nil, fmt.Errorf("getting droplet %d: %w", dropletID, err)
	}
	return dropletToInstance(droplet), nil
}

// List returns all Karpenter-managed Droplets.
func (p *DefaultProvider) List(ctx context.Context) ([]*Instance, error) {
	// List droplets by the karpenter-managed tag
	droplets, _, err := p.doClient.Droplets.ListByTag(ctx, v1alpha1.TagManagedBy, &godo.ListOptions{
		PerPage: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("listing droplets by tag: %w", err)
	}

	// Filter to only droplets belonging to this cluster
	clusterTag := v1alpha1.TagClusterPrefix + p.clusterName
	var instances []*Instance
	for i := range droplets {
		if hasTag(droplets[i].Tags, clusterTag) {
			instances = append(instances, dropletToInstance(&droplets[i]))
		}
	}
	return instances, nil
}

// dropletToInstance converts a godo Droplet to our Instance type.
func dropletToInstance(d *godo.Droplet) *Instance {
	inst := &Instance{
		ID:     d.ID,
		Name:   d.Name,
		Region: d.Region.Slug,
		Size:   d.Size.Slug,
		Status: d.Status,
		Tags:   d.Tags,
	}

	// Extract IPs
	if privateIP, err := d.PrivateIPv4(); err == nil {
		inst.PrivateIPv4 = privateIP
	}
	if publicIP, err := d.PublicIPv4(); err == nil {
		inst.PublicIPv4 = publicIP
	}

	// Image ID
	if d.Image != nil {
		inst.ImageID = d.Image.ID
	}

	// VPC UUID
	inst.VPCUUID = d.VPCUUID

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

// derefString safely dereferences a string pointer.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
