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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	fakeproviders "github.com/digitalocean/karpenter-provider-digitalocean/pkg/fake"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instance"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

// --- parseProviderID Tests ---

func TestParseProviderID(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		wantID     string
		wantErr    bool
	}{
		{
			name:       "valid provider ID - numeric",
			providerID: "digitalocean://12345",
			wantID:     "12345",
			wantErr:    false,
		},
		{
			name:       "valid provider ID - large number",
			providerID: "digitalocean://999999999",
			wantID:     "999999999",
			wantErr:    false,
		},
		{
			name:       "valid provider ID - another number",
			providerID: "digitalocean://67890",
			wantID:     "67890",
			wantErr:    false,
		},
		{
			name:       "empty provider ID",
			providerID: "",
			wantID:     "",
			wantErr:    true,
		},
		{
			name:       "missing prefix",
			providerID: "aws://us-east-1/i-12345",
			wantID:     "",
			wantErr:    true,
		},
		{
			name:       "prefix only",
			providerID: "digitalocean://",
			wantID:     "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := parseProviderID(tt.providerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseProviderID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotID != tt.wantID {
				t.Errorf("parseProviderID() = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}

// --- instanceToNodeClaim Tests ---

func TestInstanceToNodeClaimProviderID(t *testing.T) {
	cp := &CloudProvider{}

	tests := []struct {
		name      string
		dropletID string
		wantPID   string
	}{
		{
			name:      "numeric droplet ID",
			dropletID: "12345",
			wantPID:   "digitalocean://12345",
		},
		{
			name:      "large droplet ID",
			dropletID: "999999999",
			wantPID:   "digitalocean://999999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &instance.Instance{
				NodePoolID: "np-1",
				DropletID:  tt.dropletID,
				Region:     "nyc1",
				Size:       "s-1vcpu-1gb",
			}
			nc := cp.instanceToNodeClaim(inst, nil)
			if nc.Status.ProviderID != tt.wantPID {
				t.Errorf("instanceToNodeClaim() providerID = %q, want %q", nc.Status.ProviderID, tt.wantPID)
			}
		})
	}
}

func TestInstanceToNodeClaimLabels(t *testing.T) {
	cp := &CloudProvider{}

	inst := &instance.Instance{
		NodePoolID: "np-abc",
		DropletID:  "12345",
		Region:     "nyc1",
		Size:       "s-2vcpu-4gb",
	}

	nc := cp.instanceToNodeClaim(inst, nil)

	// Verify standard Kubernetes labels
	if got := nc.Labels[v1.LabelTopologyRegion]; got != "nyc1" {
		t.Errorf("expected region label %q, got %q", "nyc1", got)
	}
	if got := nc.Labels[v1.LabelInstanceTypeStable]; got != "s-2vcpu-4gb" {
		t.Errorf("expected instance type label %q, got %q", "s-2vcpu-4gb", got)
	}

	// Verify DO-specific labels
	if got := nc.Labels[v1alpha1.LabelInstanceSize]; got != "s-2vcpu-4gb" {
		t.Errorf("expected instance size label %q, got %q", "s-2vcpu-4gb", got)
	}
	if got := nc.Labels[v1alpha1.LabelRegion]; got != "nyc1" {
		t.Errorf("expected region label %q, got %q", "nyc1", got)
	}
}

func TestInstanceToNodeClaimAnnotations(t *testing.T) {
	cp := &CloudProvider{}

	inst := &instance.Instance{
		NodePoolID: "np-abc-123",
		DropletID:  "67890",
		Name:       "test-node",
		Region:     "sfo3",
		Size:       "s-2vcpu-4gb",
	}

	nc := cp.instanceToNodeClaim(inst, nil)

	// Verify node pool ID annotation
	if got := nc.Annotations[v1alpha1.AnnotationNodePoolID]; got != "np-abc-123" {
		t.Errorf("expected node pool ID annotation %q, got %q", "np-abc-123", got)
	}
	// Verify droplet ID annotation
	if got := nc.Annotations[v1alpha1.AnnotationDropletID]; got != "67890" {
		t.Errorf("expected droplet ID annotation %q, got %q", "67890", got)
	}
}

func TestInstanceToNodeClaimWithExistingClaim(t *testing.T) {
	cp := &CloudProvider{}

	existing := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-claim",
			Namespace: "default",
			Labels:    map[string]string{"existing-label": "value"},
		},
	}

	inst := &instance.Instance{
		NodePoolID: "np-1",
		DropletID:  "12345",
		Name:       "new-node",
		Region:     "nyc1",
		Size:       "s-2vcpu-4gb",
	}

	nc := cp.instanceToNodeClaim(inst, existing)

	// Should preserve existing labels and add new ones
	if nc.Labels["existing-label"] != "value" {
		t.Error("existing labels should be preserved")
	}
	if nc.Labels[v1.LabelTopologyRegion] != "nyc1" {
		t.Error("region label should be added")
	}

	// NodeName should NOT be set by instanceToNodeClaim — Karpenter's core
	// Registration controller handles this during node registration.
	if nc.Status.NodeName != "" {
		t.Errorf("expected empty node name (set by Karpenter core), got %q", nc.Status.NodeName)
	}

	// Ensure original is not modified (deep copy)
	if existing.Status.ProviderID != "" {
		t.Error("original claim should not be modified")
	}
}

// --- CloudProvider.Create Tests ---

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	// Karpenter v1 types are registered via init() to clientgoscheme.Scheme,
	// but for the fake client we register them directly into our test scheme.
	gv := schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	s.AddKnownTypes(gv,
		&karpv1.NodePool{},
		&karpv1.NodePoolList{},
		&karpv1.NodeClaim{},
		&karpv1.NodeClaimList{},
	)
	return s
}

func newTestInstanceType(name string, regions ...string) *cloudprovider.InstanceType {
	var offerings cloudprovider.Offerings
	for _, r := range regions {
		offerings = append(offerings, cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.LabelTopologyRegion, v1.NodeSelectorOpIn, r),
				scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, v1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
			),
			Price:     0.01,
			Available: true,
		})
	}
	return &cloudprovider.InstanceType{
		Name: name,
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(v1.LabelInstanceTypeStable, v1.NodeSelectorOpIn, name),
		),
		Offerings: offerings,
		Capacity: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("2"),
			v1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func TestCloudProviderCreate(t *testing.T) {
	ctx := context.Background()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
		},
	}

	scheme := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass).
		Build()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceTypeProvider := &fakeproviders.InstanceTypeProvider{
		InstanceTypes: []*cloudprovider.InstanceType{
			newTestInstanceType("s-2vcpu-4gb", "nyc1", "sfo3"),
		},
	}

	cp := New(kubeClient, instanceProvider, instanceTypeProvider)

	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-claim",
		},
		Spec: karpv1.NodeClaimSpec{
			NodeClassRef: &karpv1.NodeClassReference{
				Name:  "test-class",
				Group: v1alpha1.Group,
				Kind:  v1alpha1.DONodeClassKind,
			},
		},
	}

	result, err := cp.Create(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	// Verify provider ID format (digitalocean://<dropletID>)
	if result.Status.ProviderID == "" {
		t.Error("expected non-empty provider ID")
	}

	// Verify the instance was created in the fake
	if instanceProvider.CreateCalls != 1 {
		t.Errorf("expected 1 create call, got %d", instanceProvider.CreateCalls)
	}

	// Verify annotations contain node pool ID
	if result.Annotations[v1alpha1.AnnotationNodePoolID] == "" {
		t.Error("expected non-empty node pool ID annotation")
	}
}

func TestCloudProviderCreateMissingNodeClass(t *testing.T) {
	ctx := context.Background()

	scheme := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceTypeProvider := &fakeproviders.InstanceTypeProvider{}

	cp := New(kubeClient, instanceProvider, instanceTypeProvider)

	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-claim",
		},
		Spec: karpv1.NodeClaimSpec{
			NodeClassRef: &karpv1.NodeClassReference{
				Name:  "nonexistent-class",
				Group: v1alpha1.Group,
				Kind:  v1alpha1.DONodeClassKind,
			},
		},
	}

	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error when node class doesn't exist")
	}
}

func TestCloudProviderCreateInstanceProviderError(t *testing.T) {
	ctx := context.Background()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
		},
	}

	scheme := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass).
		Build()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.CreateError = fmt.Errorf("API rate limit exceeded")
	instanceTypeProvider := &fakeproviders.InstanceTypeProvider{
		InstanceTypes: []*cloudprovider.InstanceType{
			newTestInstanceType("s-2vcpu-4gb", "nyc1"),
		},
	}

	cp := New(kubeClient, instanceProvider, instanceTypeProvider)

	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-claim"},
		Spec: karpv1.NodeClaimSpec{
			NodeClassRef: &karpv1.NodeClassReference{
				Name:  "test-class",
				Group: v1alpha1.Group,
				Kind:  v1alpha1.DONodeClassKind,
			},
		},
	}

	_, err := cp.Create(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error from instance provider")
	}
}

// --- CloudProvider.Delete Tests ---

func TestCloudProviderDeleteWithAnnotation(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.NodePools["nodepool-1"] = &instance.Instance{
		NodePoolID: "nodepool-1",
		DropletID:  "12345",
		Region:     "nyc1",
		Size:       "s-1vcpu-1gb",
	}

	cp := New(nil, instanceProvider, nil)

	// Delete using the AnnotationNodePoolID (preferred path)
	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				v1alpha1.AnnotationNodePoolID: "nodepool-1",
			},
		},
		Status: karpv1.NodeClaimStatus{
			ProviderID: "digitalocean://12345",
		},
	}

	err := cp.Delete(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	if instanceProvider.DeleteCalls != 1 {
		t.Errorf("expected 1 delete call, got %d", instanceProvider.DeleteCalls)
	}
	if len(instanceProvider.NodePools) != 0 {
		t.Errorf("expected 0 node pools after delete, got %d", len(instanceProvider.NodePools))
	}
}

func TestCloudProviderDeleteFallbackToProviderID(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.NodePools["nodepool-1"] = &instance.Instance{
		NodePoolID: "nodepool-1",
		DropletID:  "12345",
		Region:     "nyc1",
		Size:       "s-1vcpu-1gb",
	}

	cp := New(nil, instanceProvider, nil)

	// Delete using just the provider ID (fallback path: parse → Get → Delete)
	nodeClaim := &karpv1.NodeClaim{
		Status: karpv1.NodeClaimStatus{
			ProviderID: "digitalocean://12345",
		},
	}

	err := cp.Delete(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	if instanceProvider.DeleteCalls != 1 {
		t.Errorf("expected 1 delete call, got %d", instanceProvider.DeleteCalls)
	}
	if len(instanceProvider.NodePools) != 0 {
		t.Errorf("expected 0 node pools after delete, got %d", len(instanceProvider.NodePools))
	}
}

func TestCloudProviderDeleteInvalidProviderID(t *testing.T) {
	ctx := context.Background()
	cp := New(nil, fakeproviders.NewInstanceProvider(), nil)

	nodeClaim := &karpv1.NodeClaim{
		Status: karpv1.NodeClaimStatus{
			ProviderID: "invalid-id",
		},
	}

	err := cp.Delete(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error with invalid provider ID")
	}
}

func TestCloudProviderDeleteEmptyProviderID(t *testing.T) {
	ctx := context.Background()
	cp := New(nil, fakeproviders.NewInstanceProvider(), nil)

	nodeClaim := &karpv1.NodeClaim{
		Status: karpv1.NodeClaimStatus{
			ProviderID: "",
		},
	}

	err := cp.Delete(ctx, nodeClaim)
	if err == nil {
		t.Fatal("expected error with empty provider ID")
	}
}

// --- CloudProvider.Get Tests ---

func TestCloudProviderGet(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.NodePools["nodepool-1"] = &instance.Instance{
		NodePoolID: "nodepool-1",
		DropletID:  "12345",
		Name:       "test-node",
		Region:     "nyc1",
		Size:       "s-2vcpu-4gb",
	}

	cp := New(nil, instanceProvider, nil)

	nc, err := cp.Get(ctx, "digitalocean://12345")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}

	expectedPID := "digitalocean://12345"
	if nc.Status.ProviderID != expectedPID {
		t.Errorf("expected provider ID %q, got %q", expectedPID, nc.Status.ProviderID)
	}
	if nc.Labels[v1.LabelInstanceTypeStable] != "s-2vcpu-4gb" {
		t.Errorf("expected instance type label %q, got %q", "s-2vcpu-4gb", nc.Labels[v1.LabelInstanceTypeStable])
	}
	if nc.Annotations[v1alpha1.AnnotationNodePoolID] != "nodepool-1" {
		t.Errorf("expected node pool ID annotation %q, got %q", "nodepool-1", nc.Annotations[v1alpha1.AnnotationNodePoolID])
	}
}

func TestCloudProviderGetNotFound(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	cp := New(nil, instanceProvider, nil)

	_, err := cp.Get(ctx, "digitalocean://99999")
	if err == nil {
		t.Fatal("expected error for non-existent instance")
	}
}

// --- CloudProvider.List Tests ---

func TestCloudProviderList(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.NodePools["np-1"] = &instance.Instance{
		NodePoolID: "np-1", DropletID: "100", Region: "nyc1", Size: "s-1vcpu-1gb",
	}
	instanceProvider.NodePools["np-2"] = &instance.Instance{
		NodePoolID: "np-2", DropletID: "200", Region: "sfo3", Size: "s-2vcpu-4gb",
	}
	instanceProvider.NodePools["np-3"] = &instance.Instance{
		NodePoolID: "np-3", DropletID: "300", Region: "ams3", Size: "g-2vcpu-8gb",
	}

	cp := New(nil, instanceProvider, nil)

	claims, err := cp.List(ctx)
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}

	if len(claims) != 3 {
		t.Errorf("expected 3 node claims, got %d", len(claims))
	}
}

func TestCloudProviderListEmpty(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	cp := New(nil, instanceProvider, nil)

	claims, err := cp.List(ctx)
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}

	if len(claims) != 0 {
		t.Errorf("expected 0 node claims, got %d", len(claims))
	}
}

func TestCloudProviderListError(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.ListError = fmt.Errorf("API error")
	cp := New(nil, instanceProvider, nil)

	_, err := cp.List(ctx)
	if err == nil {
		t.Fatal("expected error from instance provider")
	}
}

// --- CloudProvider.GetInstanceTypes Tests ---

func TestCloudProviderGetInstanceTypes(t *testing.T) {
	ctx := context.Background()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
		},
	}

	scheme := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass).
		Build()

	expectedTypes := []*cloudprovider.InstanceType{
		newTestInstanceType("s-1vcpu-1gb", "nyc1"),
		newTestInstanceType("s-2vcpu-4gb", "nyc1"),
	}
	instanceTypeProvider := &fakeproviders.InstanceTypeProvider{
		InstanceTypes: expectedTypes,
	}

	cp := New(kubeClient, nil, instanceTypeProvider)

	nodePool := &karpv1.NodePool{
		Spec: karpv1.NodePoolSpec{
			Template: karpv1.NodeClaimTemplate{
				Spec: karpv1.NodeClaimTemplateSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Name:  "test-class",
						Group: v1alpha1.Group,
						Kind:  v1alpha1.DONodeClassKind,
					},
				},
			},
		},
	}

	types, err := cp.GetInstanceTypes(ctx, nodePool)
	if err != nil {
		t.Fatalf("GetInstanceTypes() unexpected error: %v", err)
	}

	if len(types) != 2 {
		t.Errorf("expected 2 instance types, got %d", len(types))
	}
}

// --- CloudProvider.IsDrifted Tests ---

func TestCloudProviderIsDrifted(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		nodeClass *v1alpha1.DONodeClass
		instance  *instance.Instance
		labels    map[string]string // extra labels on the NodeClaim
		wantDrift cloudprovider.DriftReason
		wantErr   bool
	}{
		{
			name: "no drift",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
				},
			},
			instance: &instance.Instance{
				NodePoolID: "np-1",
				DropletID:  "12345",
				Region:     "nyc1",
				Size:       "s-2vcpu-4gb",
			},
			labels: map[string]string{
				v1.LabelInstanceTypeStable: "s-2vcpu-4gb",
			},
			wantDrift: "",
		},
		{
			name: "region drift",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
				},
			},
			instance: &instance.Instance{
				NodePoolID: "np-1",
				DropletID:  "12345",
				Region:     "sfo3", // Different from nodeClass.Spec.Region
				Size:       "s-2vcpu-4gb",
			},
			wantDrift: DriftReasonRegionChanged,
		},
		{
			name: "node pool size drift",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
				},
			},
			instance: &instance.Instance{
				NodePoolID: "np-1",
				DropletID:  "12345",
				Region:     "nyc1",
				Size:       "s-4vcpu-8gb", // Different from NodeClaim label
			},
			labels: map[string]string{
				v1.LabelInstanceTypeStable: "s-2vcpu-4gb",
			},
			wantDrift: DriftReasonNodePoolChanged,
		},
		{
			name: "no drift when no instance type label on claim",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
				},
			},
			instance: &instance.Instance{
				NodePoolID: "np-1",
				DropletID:  "12345",
				Region:     "nyc1",
				Size:       "s-4vcpu-8gb",
			},
			// No labels → no size check → no drift
			wantDrift: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			kubeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.nodeClass).
				Build()

			instanceProvider := fakeproviders.NewInstanceProvider()
			instanceProvider.NodePools["np-1"] = tt.instance

			cp := New(kubeClient, instanceProvider, nil)

			nodeClaim := &karpv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.labels,
				},
				Spec: karpv1.NodeClaimSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Name:  tt.nodeClass.Name,
						Group: v1alpha1.Group,
						Kind:  v1alpha1.DONodeClassKind,
					},
				},
				Status: karpv1.NodeClaimStatus{
					ProviderID: fmt.Sprintf("digitalocean://%s", tt.instance.DropletID),
				},
			}

			drift, err := cp.IsDrifted(ctx, nodeClaim)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsDrifted() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if drift != tt.wantDrift {
				t.Errorf("IsDrifted() = %q, want %q", drift, tt.wantDrift)
			}
		})
	}
}

// --- CloudProvider metadata Tests ---

func TestCloudProviderName(t *testing.T) {
	cp := &CloudProvider{}
	if cp.Name() != "digitalocean" {
		t.Errorf("expected name %q, got %q", "digitalocean", cp.Name())
	}
}

func TestCloudProviderGetSupportedNodeClasses(t *testing.T) {
	cp := &CloudProvider{}
	classes := cp.GetSupportedNodeClasses()
	if len(classes) != 1 {
		t.Fatalf("expected 1 supported node class, got %d", len(classes))
	}
	_, ok := classes[0].(*v1alpha1.DONodeClass)
	if !ok {
		t.Error("expected DONodeClass as supported node class")
	}
}

func TestCloudProviderRepairPolicies(t *testing.T) {
	cp := &CloudProvider{}
	policies := cp.RepairPolicies()
	if policies != nil {
		t.Errorf("expected nil repair policies, got %v", policies)
	}
}

// --- Roundtrip test: Create → Get → Delete ---

func TestCloudProviderCreateGetDeleteRoundtrip(t *testing.T) {
	ctx := context.Background()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
		},
	}

	scheme := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass).
		Build()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceTypeProvider := &fakeproviders.InstanceTypeProvider{
		InstanceTypes: []*cloudprovider.InstanceType{
			newTestInstanceType("s-2vcpu-4gb", "nyc1"),
		},
	}

	cp := New(kubeClient, instanceProvider, instanceTypeProvider)

	// 1. Create
	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "roundtrip-claim"},
		Spec: karpv1.NodeClaimSpec{
			NodeClassRef: &karpv1.NodeClassReference{
				Name:  "test-class",
				Group: v1alpha1.Group,
				Kind:  v1alpha1.DONodeClassKind,
			},
		},
	}

	created, err := cp.Create(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Verify annotations were set
	nodePoolID := created.Annotations[v1alpha1.AnnotationNodePoolID]
	if nodePoolID == "" {
		t.Fatal("expected non-empty node pool ID annotation after create")
	}

	// 2. Get
	got, err := cp.Get(ctx, created.Status.ProviderID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.Status.ProviderID != created.Status.ProviderID {
		t.Errorf("Get() returned different provider ID: %q vs %q", got.Status.ProviderID, created.Status.ProviderID)
	}

	// 3. List — should have 1
	listed, err := cp.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("expected 1 listed, got %d", len(listed))
	}

	// 4. Delete using annotation (preferred path)
	deleteNC := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				v1alpha1.AnnotationNodePoolID: nodePoolID,
			},
		},
		Status: karpv1.NodeClaimStatus{
			ProviderID: created.Status.ProviderID,
		},
	}
	err = cp.Delete(ctx, deleteNC)
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// 5. List — should have 0
	listed, err = cp.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("expected 0 listed after delete, got %d", len(listed))
	}
}

// --- Provider ID parsing stress test ---

func TestParseProviderIDStress(t *testing.T) {
	// Test with various droplet ID formats
	for _, id := range []string{"1", "999999999", "2147483647", "100000"} {
		providerID := fmt.Sprintf("digitalocean://%s", id)
		gotID, err := parseProviderID(providerID)
		if err != nil {
			t.Errorf("parseProviderID(%q) unexpected error: %v", providerID, err)
		}
		if gotID != id {
			t.Errorf("parseProviderID(%q) = %q, want %q", providerID, gotID, id)
		}
	}
}
