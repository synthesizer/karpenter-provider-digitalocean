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

package cloudprovider

import (
	"context"
	"fmt"
	"strings"

	"github.com/awslabs/operatorpkg/status"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instance"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instancetype"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

var _ cloudprovider.CloudProvider = (*CloudProvider)(nil)

// CloudProvider implements the Karpenter CloudProvider interface for DigitalOcean.
// It manages DOKS node pools (one per NodeClaim) for node provisioning.
type CloudProvider struct {
	kubeClient           client.Client
	instanceProvider     instance.Provider
	instanceTypeProvider instancetype.Provider
}

// New creates a new DigitalOcean CloudProvider.
func New(
	kubeClient client.Client,
	instanceProvider instance.Provider,
	instanceTypeProvider instancetype.Provider,
) *CloudProvider {
	return &CloudProvider{
		kubeClient:           kubeClient,
		instanceProvider:     instanceProvider,
		instanceTypeProvider: instanceTypeProvider,
	}
}

// Create launches a new DOKS node pool for the given NodeClaim.
// Each NodeClaim maps to a single DOKS node pool with Count=1.
func (c *CloudProvider) Create(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*karpv1.NodeClaim, error) {
	// 1. Resolve the DONodeClass from the nodeClassRef
	nodeClass, err := c.resolveNodeClass(ctx, nodeClaim)
	if err != nil {
		return nil, fmt.Errorf("resolving node class: %w", err)
	}

	// 2. Get compatible instance types
	instanceTypes, err := c.instanceTypeProvider.List(ctx, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("listing instance types: %w", err)
	}

	// 3. Create the DOKS node pool
	created, err := c.instanceProvider.Create(ctx, nodeClass, nodeClaim, instanceTypes)
	if err != nil {
		return nil, fmt.Errorf("creating instance: %w", err)
	}

	// 4. Convert the instance to a NodeClaim
	return c.instanceToNodeClaim(created, nodeClaim), nil
}

// Delete terminates the DOKS node pool associated with the given NodeClaim.
// It reads the node pool ID from the NodeClaim annotation to perform an
// efficient direct deletion.
func (c *CloudProvider) Delete(ctx context.Context, nodeClaim *karpv1.NodeClaim) error {
	// Try to get the node pool ID from annotation first (most efficient)
	if nodePoolID, ok := nodeClaim.Annotations[v1alpha1.AnnotationNodePoolID]; ok && nodePoolID != "" {
		return c.instanceProvider.Delete(ctx, nodePoolID)
	}

	// Fallback: find the node pool by droplet ID
	dropletID, err := parseProviderID(nodeClaim.Status.ProviderID)
	if err != nil {
		return fmt.Errorf("parsing provider ID: %w", err)
	}

	inst, err := c.instanceProvider.Get(ctx, dropletID)
	if err != nil {
		return fmt.Errorf("getting instance for deletion: %w", err)
	}

	return c.instanceProvider.Delete(ctx, inst.NodePoolID)
}

// Get returns a NodeClaim for the given provider ID.
func (c *CloudProvider) Get(ctx context.Context, providerID string) (*karpv1.NodeClaim, error) {
	dropletID, err := parseProviderID(providerID)
	if err != nil {
		return nil, fmt.Errorf("parsing provider ID: %w", err)
	}
	inst, err := c.instanceProvider.Get(ctx, dropletID)
	if err != nil {
		return nil, fmt.Errorf("getting instance: %w", err)
	}
	return c.instanceToNodeClaim(inst, nil), nil
}

// List returns all NodeClaims managed by this provider.
func (c *CloudProvider) List(ctx context.Context) ([]*karpv1.NodeClaim, error) {
	instances, err := c.instanceProvider.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing instances: %w", err)
	}
	var nodeClaims []*karpv1.NodeClaim
	for _, inst := range instances {
		nodeClaims = append(nodeClaims, c.instanceToNodeClaim(inst, nil))
	}
	return nodeClaims, nil
}

// GetInstanceTypes returns the instance types available for the given NodePool.
func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *karpv1.NodePool) ([]*cloudprovider.InstanceType, error) {
	nodeClass, err := c.resolveNodeClassFromPool(ctx, nodePool)
	if err != nil {
		return nil, fmt.Errorf("resolving node class from pool: %w", err)
	}
	return c.instanceTypeProvider.List(ctx, nodeClass)
}

// GetSupportedNodeClasses returns the NodeClass types supported by this provider.
func (c *CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{&v1alpha1.DONodeClass{}}
}

// RepairPolicies returns the repair policies supported by this provider.
// DigitalOcean does not currently support custom repair policies.
func (c *CloudProvider) RepairPolicies() []cloudprovider.RepairPolicy {
	return nil
}

// Name returns the name of this cloud provider.
func (c *CloudProvider) Name() string {
	return "digitalocean"
}

// Drift reasons reported by the DigitalOcean cloud provider.
const (
	DriftReasonRegionChanged   cloudprovider.DriftReason = "RegionChanged"
	DriftReasonNodePoolChanged cloudprovider.DriftReason = "NodePoolChanged"
)

// IsDrifted checks if the given NodeClaim has drifted from its desired state.
func (c *CloudProvider) IsDrifted(ctx context.Context, nodeClaim *karpv1.NodeClaim) (cloudprovider.DriftReason, error) {
	// Resolve the DONodeClass from the nodeClassRef
	nodeClass, err := c.resolveNodeClass(ctx, nodeClaim)
	if err != nil {
		return "", fmt.Errorf("resolving node class for drift check: %w", err)
	}

	// Parse provider ID to get the droplet
	dropletID, err := parseProviderID(nodeClaim.Status.ProviderID)
	if err != nil {
		return "", fmt.Errorf("parsing provider ID for drift check: %w", err)
	}

	inst, err := c.instanceProvider.Get(ctx, dropletID)
	if err != nil {
		return "", fmt.Errorf("getting instance for drift check: %w", err)
	}

	// Check region drift — if the instance is not in the expected region.
	if inst.Region != nodeClass.Spec.Region {
		return DriftReasonRegionChanged, nil
	}

	// Check size drift — if the node pool size doesn't match what the NodeClaim expects.
	if expectedSize, ok := nodeClaim.Labels[v1.LabelInstanceTypeStable]; ok {
		if inst.Size != expectedSize {
			return DriftReasonNodePoolChanged, nil
		}
	}

	return "", nil
}

// resolveNodeClass fetches the DONodeClass referenced by the NodeClaim.
func (c *CloudProvider) resolveNodeClass(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*v1alpha1.DONodeClass, error) {
	nodeClass := &v1alpha1.DONodeClass{}
	if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: nodeClaim.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, fmt.Errorf("getting DONodeClass %q: %w", nodeClaim.Spec.NodeClassRef.Name, err)
	}
	return nodeClass, nil
}

// resolveNodeClassFromPool fetches the DONodeClass referenced by the NodePool.
func (c *CloudProvider) resolveNodeClassFromPool(ctx context.Context, nodePool *karpv1.NodePool) (*v1alpha1.DONodeClass, error) {
	nodeClass := &v1alpha1.DONodeClass{}
	if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: nodePool.Spec.Template.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, fmt.Errorf("getting DONodeClass %q: %w", nodePool.Spec.Template.Spec.NodeClassRef.Name, err)
	}
	return nodeClass, nil
}

// providerIDPrefix is the scheme used for DigitalOcean provider IDs.
// DOKS sets the kubelet provider ID to "digitalocean://<dropletID>".
const providerIDPrefix = "digitalocean://"

// instanceToNodeClaim converts a DigitalOcean DOKS instance to a Karpenter NodeClaim.
func (c *CloudProvider) instanceToNodeClaim(inst *instance.Instance, existingClaim *karpv1.NodeClaim) *karpv1.NodeClaim {
	nodeClaim := &karpv1.NodeClaim{}
	if existingClaim != nil {
		nodeClaim = existingClaim.DeepCopy()
	}

	// Set the provider ID to match what the DOKS kubelet registers:
	// digitalocean://<dropletID>
	nodeClaim.Status.ProviderID = fmt.Sprintf("%s%s", providerIDPrefix, inst.DropletID)

	// Set labels from the instance
	if nodeClaim.Labels == nil {
		nodeClaim.Labels = make(map[string]string)
	}
	nodeClaim.Labels[v1.LabelTopologyRegion] = inst.Region
	nodeClaim.Labels[v1.LabelInstanceTypeStable] = inst.Size
	nodeClaim.Labels[v1alpha1.LabelInstanceSize] = inst.Size
	nodeClaim.Labels[v1alpha1.LabelRegion] = inst.Region

	// Set node name
	nodeClaim.Status.NodeName = inst.Name

	// Store node pool ID and droplet ID as annotations for efficient operations
	if nodeClaim.Annotations == nil {
		nodeClaim.Annotations = make(map[string]string)
	}
	nodeClaim.Annotations[v1alpha1.AnnotationNodePoolID] = inst.NodePoolID
	nodeClaim.Annotations[v1alpha1.AnnotationDropletID] = inst.DropletID

	// Set capacity (will be populated with real values once the node registers)
	if nodeClaim.Status.Capacity == nil {
		nodeClaim.Status.Capacity = v1.ResourceList{}
	}

	return nodeClaim
}

// parseProviderID extracts the droplet ID from a provider ID string.
// DOKS uses the format: digitalocean://<dropletID>
func parseProviderID(providerID string) (string, error) {
	if providerID == "" {
		return "", fmt.Errorf("provider ID is empty")
	}

	if !strings.HasPrefix(providerID, providerIDPrefix) {
		return "", fmt.Errorf("provider ID %q does not have expected prefix %q", providerID, providerIDPrefix)
	}

	// Remove the prefix to get the droplet ID
	dropletID := strings.TrimPrefix(providerID, providerIDPrefix)
	if dropletID == "" {
		return "", fmt.Errorf("provider ID %q has empty droplet ID", providerID)
	}

	return dropletID, nil
}
