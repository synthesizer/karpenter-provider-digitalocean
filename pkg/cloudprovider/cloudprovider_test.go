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
	"strconv"
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
			name:       "valid provider ID",
			providerID: "digitalocean://nyc1/12345",
			wantID:     "12345",
			wantErr:    false,
		},
		{
			name:       "valid provider ID different region",
			providerID: "digitalocean://sfo3/67890",
			wantID:     "67890",
			wantErr:    false,
		},
		{
			name:       "valid provider ID large droplet ID",
			providerID: "digitalocean://ams3/999999999",
			wantID:     "999999999",
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
			name:       "malformed - no region",
			providerID: "digitalocean://12345",
			wantID:     "",
			wantErr:    true,
		},
		{
			name:       "malformed - empty region",
			providerID: "digitalocean:///12345",
			wantID:     "",
			wantErr:    true,
		},
		{
			name:       "malformed - empty ID",
			providerID: "digitalocean://nyc1/",
			wantID:     "",
			wantErr:    true,
		},
		{
			name:       "malformed - non-numeric ID",
			providerID: "digitalocean://nyc1/abc",
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
		name    string
		region  string
		id      int
		wantPID string
	}{
		{
			name:    "nyc1 region",
			region:  "nyc1",
			id:      12345,
			wantPID: "digitalocean://nyc1/12345",
		},
		{
			name:    "sfo3 region",
			region:  "sfo3",
			id:      67890,
			wantPID: "digitalocean://sfo3/67890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &instance.Instance{
				ID:     tt.id,
				Region: tt.region,
				Size:   "s-1vcpu-1gb",
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
		ID:      12345,
		Region:  "nyc1",
		Size:    "s-2vcpu-4gb",
		ImageID: 99999,
		VPCUUID: "vpc-abc-123",
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
	if got := nc.Labels[v1alpha1.LabelImageID]; got != "99999" {
		t.Errorf("expected image ID label %q, got %q", "99999", got)
	}
	if got := nc.Labels[v1alpha1.LabelVPCUUID]; got != "vpc-abc-123" {
		t.Errorf("expected VPC UUID label %q, got %q", "vpc-abc-123", got)
	}
}

func TestInstanceToNodeClaimAddresses(t *testing.T) {
	cp := &CloudProvider{}

	tests := []struct {
		name          string
		inst          *instance.Instance
		wantPrivateIP string
		wantPublicIP  string
	}{
		{
			name: "both private and public IPs",
			inst: &instance.Instance{
				ID:          1,
				Region:      "nyc1",
				Size:        "s-1vcpu-1gb",
				PrivateIPv4: "10.0.0.2",
				PublicIPv4:  "1.2.3.4",
			},
			wantPrivateIP: "10.0.0.2",
			wantPublicIP:  "1.2.3.4",
		},
		{
			name: "private IP only",
			inst: &instance.Instance{
				ID:          2,
				Region:      "nyc1",
				Size:        "s-1vcpu-1gb",
				PrivateIPv4: "10.0.0.2",
			},
			wantPrivateIP: "10.0.0.2",
		},
		{
			name: "no IPs",
			inst: &instance.Instance{
				ID:     3,
				Region: "nyc1",
				Size:   "s-1vcpu-1gb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nc := cp.instanceToNodeClaim(tt.inst, nil)
			if nc.Status.ProviderID == "" {
				t.Error("expected non-empty provider ID")
			}
			gotPrivate := nc.Annotations[v1alpha1.AnnotationPrivateIPv4]
			if gotPrivate != tt.wantPrivateIP {
				t.Errorf("private IP annotation = %q, want %q", gotPrivate, tt.wantPrivateIP)
			}
			gotPublic := nc.Annotations[v1alpha1.AnnotationPublicIPv4]
			if gotPublic != tt.wantPublicIP {
				t.Errorf("public IP annotation = %q, want %q", gotPublic, tt.wantPublicIP)
			}
		})
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
		ID:          12345,
		Name:        "new-node",
		Region:      "nyc1",
		Size:        "s-2vcpu-4gb",
		PrivateIPv4: "10.0.0.2",
	}

	nc := cp.instanceToNodeClaim(inst, existing)

	// Should preserve existing labels and add new ones
	if nc.Labels["existing-label"] != "value" {
		t.Error("existing labels should be preserved")
	}
	if nc.Labels[v1.LabelTopologyRegion] != "nyc1" {
		t.Error("region label should be added")
	}

	// Should use the instance name as node name
	if nc.Status.NodeName != "new-node" {
		t.Errorf("expected node name %q, got %q", "new-node", nc.Status.NodeName)
	}

	// Ensure original is not modified (deep copy)
	if existing.Status.ProviderID != "" {
		t.Error("original claim should not be modified")
	}
}

func TestInstanceToNodeClaimMissingOptionalFields(t *testing.T) {
	cp := &CloudProvider{}

	inst := &instance.Instance{
		ID:     12345,
		Region: "nyc1",
		Size:   "s-1vcpu-1gb",
		// No ImageID, no VPCUUID
	}

	nc := cp.instanceToNodeClaim(inst, nil)

	// ImageID label should not be present when ImageID is 0
	if _, ok := nc.Labels[v1alpha1.LabelImageID]; ok {
		t.Error("ImageID label should not be set when ImageID is 0")
	}
	// VPCUUID label should not be present when empty
	if _, ok := nc.Labels[v1alpha1.LabelVPCUUID]; ok {
		t.Error("VPCUUID label should not be set when empty")
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
			Image:  v1alpha1.DONodeClassImage{ID: 12345},
		},
		Status: v1alpha1.DONodeClassStatus{
			ImageID: 12345,
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
	imageProvider := fakeproviders.NewImageProvider()

	cp := New(kubeClient, instanceProvider, instanceTypeProvider, imageProvider)

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

	// Verify provider ID format
	if result.Status.ProviderID == "" {
		t.Error("expected non-empty provider ID")
	}

	// Verify the instance was created in the fake
	if instanceProvider.CreateCalls != 1 {
		t.Errorf("expected 1 create call, got %d", instanceProvider.CreateCalls)
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
	imageProvider := fakeproviders.NewImageProvider()

	cp := New(kubeClient, instanceProvider, instanceTypeProvider, imageProvider)

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
			Image:  v1alpha1.DONodeClassImage{ID: 12345},
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
	imageProvider := fakeproviders.NewImageProvider()

	cp := New(kubeClient, instanceProvider, instanceTypeProvider, imageProvider)

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

func TestCloudProviderDelete(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.Instances[12345] = &instance.Instance{
		ID: 12345, Region: "nyc1", Size: "s-1vcpu-1gb",
	}

	cp := New(nil, instanceProvider, nil, nil)

	nodeClaim := &karpv1.NodeClaim{
		Status: karpv1.NodeClaimStatus{
			ProviderID: "digitalocean://nyc1/12345",
		},
	}

	err := cp.Delete(ctx, nodeClaim)
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	if instanceProvider.DeleteCalls != 1 {
		t.Errorf("expected 1 delete call, got %d", instanceProvider.DeleteCalls)
	}
	if len(instanceProvider.Instances) != 0 {
		t.Errorf("expected 0 instances after delete, got %d", len(instanceProvider.Instances))
	}
}

func TestCloudProviderDeleteInvalidProviderID(t *testing.T) {
	ctx := context.Background()
	cp := New(nil, fakeproviders.NewInstanceProvider(), nil, nil)

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
	cp := New(nil, fakeproviders.NewInstanceProvider(), nil, nil)

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
	instanceProvider.Instances[12345] = &instance.Instance{
		ID:          12345,
		Name:        "test-node",
		Region:      "nyc1",
		Size:        "s-2vcpu-4gb",
		PrivateIPv4: "10.0.0.2",
		ImageID:     99999,
	}

	cp := New(nil, instanceProvider, nil, nil)

	nc, err := cp.Get(ctx, "digitalocean://nyc1/12345")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}

	expectedPID := "digitalocean://nyc1/12345"
	if nc.Status.ProviderID != expectedPID {
		t.Errorf("expected provider ID %q, got %q", expectedPID, nc.Status.ProviderID)
	}
	if nc.Labels[v1.LabelInstanceTypeStable] != "s-2vcpu-4gb" {
		t.Errorf("expected instance type label %q, got %q", "s-2vcpu-4gb", nc.Labels[v1.LabelInstanceTypeStable])
	}
}

func TestCloudProviderGetNotFound(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	cp := New(nil, instanceProvider, nil, nil)

	_, err := cp.Get(ctx, "digitalocean://nyc1/99999")
	if err == nil {
		t.Fatal("expected error for non-existent instance")
	}
}

// --- CloudProvider.List Tests ---

func TestCloudProviderList(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.Instances[100] = &instance.Instance{
		ID: 100, Region: "nyc1", Size: "s-1vcpu-1gb",
	}
	instanceProvider.Instances[200] = &instance.Instance{
		ID: 200, Region: "sfo3", Size: "s-2vcpu-4gb",
	}
	instanceProvider.Instances[300] = &instance.Instance{
		ID: 300, Region: "ams3", Size: "g-2vcpu-8gb",
	}

	cp := New(nil, instanceProvider, nil, nil)

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
	cp := New(nil, instanceProvider, nil, nil)

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
	cp := New(nil, instanceProvider, nil, nil)

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
			Image:  v1alpha1.DONodeClassImage{ID: 12345},
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

	cp := New(kubeClient, nil, instanceTypeProvider, nil)

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
		wantDrift cloudprovider.DriftReason
		wantErr   bool
	}{
		{
			name: "no drift",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region:  "nyc1",
					VPCUUID: "vpc-123",
					Image:   v1alpha1.DONodeClassImage{ID: 12345},
				},
				Status: v1alpha1.DONodeClassStatus{ImageID: 99999},
			},
			instance: &instance.Instance{
				ID:      12345,
				Region:  "nyc1",
				Size:    "s-2vcpu-4gb",
				ImageID: 99999,
				VPCUUID: "vpc-123",
			},
			wantDrift: "",
		},
		{
			name: "image drift",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
					Image:  v1alpha1.DONodeClassImage{ID: 12345},
				},
				Status: v1alpha1.DONodeClassStatus{ImageID: 99999},
			},
			instance: &instance.Instance{
				ID:      12345,
				Region:  "nyc1",
				Size:    "s-2vcpu-4gb",
				ImageID: 88888, // Different from nodeClass.Status.ImageID
			},
			wantDrift: DriftReasonImageChanged,
		},
		{
			name: "region drift",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
					Image:  v1alpha1.DONodeClassImage{ID: 12345},
				},
			},
			instance: &instance.Instance{
				ID:     12345,
				Region: "sfo3", // Different from nodeClass.Spec.Region
				Size:   "s-2vcpu-4gb",
			},
			wantDrift: DriftReasonRegionChanged,
		},
		{
			name: "VPC drift",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region:  "nyc1",
					VPCUUID: "vpc-123",
					Image:   v1alpha1.DONodeClassImage{ID: 12345},
				},
			},
			instance: &instance.Instance{
				ID:      12345,
				Region:  "nyc1",
				Size:    "s-2vcpu-4gb",
				VPCUUID: "vpc-999", // Different from nodeClass.Spec.VPCUUID
			},
			wantDrift: DriftReasonVPCChanged,
		},
		{
			name: "no drift with empty VPC spec",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
					Image:  v1alpha1.DONodeClassImage{ID: 12345},
					// No VPCUUID specified
				},
			},
			instance: &instance.Instance{
				ID:      12345,
				Region:  "nyc1",
				Size:    "s-2vcpu-4gb",
				VPCUUID: "vpc-whatever",
			},
			wantDrift: "",
		},
		{
			name: "no drift when imageID is zero in status",
			nodeClass: &v1alpha1.DONodeClass{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec: v1alpha1.DONodeClassSpec{
					Region: "nyc1",
					Image:  v1alpha1.DONodeClassImage{Slug: "ubuntu-24-04-x64"},
				},
				Status: v1alpha1.DONodeClassStatus{
					ImageID: 0, // Not resolved yet
				},
			},
			instance: &instance.Instance{
				ID:      12345,
				Region:  "nyc1",
				Size:    "s-2vcpu-4gb",
				ImageID: 99999,
			},
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
			instanceProvider.Instances[tt.instance.ID] = tt.instance

			cp := New(kubeClient, instanceProvider, nil, nil)

			nodeClaim := &karpv1.NodeClaim{
				Spec: karpv1.NodeClaimSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Name:  tt.nodeClass.Name,
						Group: v1alpha1.Group,
						Kind:  v1alpha1.DONodeClassKind,
					},
				},
				Status: karpv1.NodeClaimStatus{
					ProviderID: fmt.Sprintf("digitalocean://%s/%d", tt.instance.Region, tt.instance.ID),
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
			Image:  v1alpha1.DONodeClassImage{ID: 12345},
		},
		Status: v1alpha1.DONodeClassStatus{ImageID: 12345},
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
	imageProvider := fakeproviders.NewImageProvider()

	cp := New(kubeClient, instanceProvider, instanceTypeProvider, imageProvider)

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

	// 4. Delete
	deleteNC := &karpv1.NodeClaim{
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
	// Test with very large droplet IDs
	for _, id := range []int{1, 999999999, 2147483647} {
		providerID := fmt.Sprintf("digitalocean://nyc1/%d", id)
		gotID, err := parseProviderID(providerID)
		if err != nil {
			t.Errorf("parseProviderID(%q) unexpected error: %v", providerID, err)
		}
		if gotID != strconv.Itoa(id) {
			t.Errorf("parseProviderID(%q) = %q, want %q", providerID, gotID, strconv.Itoa(id))
		}
	}
}
