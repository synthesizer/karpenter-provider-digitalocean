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

package nodeclaim

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fakeproviders "github.com/digitalocean/karpenter-provider-digitalocean/pkg/fake"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instance"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	gv := schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	s.AddKnownTypes(gv,
		&karpv1.NodePool{},
		&karpv1.NodePoolList{},
		&karpv1.NodeClaim{},
		&karpv1.NodeClaimList{},
	)
	metav1.AddToGroupVersion(s, gv)
	return s
}

func TestGCDeletesOrphanedDroplets(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()

	// Add some instances — one has a matching NodeClaim, one doesn't
	instanceProvider.Instances[100] = &instance.Instance{
		ID:     100,
		Name:   "orphan-node",
		Region: "nyc1",
		Size:   "s-1vcpu-1gb",
		Status: "active",
	}
	instanceProvider.Instances[200] = &instance.Instance{
		ID:     200,
		Name:   "claimed-node",
		Region: "nyc1",
		Size:   "s-2vcpu-4gb",
		Status: "active",
	}

	// Create a NodeClaim that matches instance 200
	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "claimed-node",
		},
		Status: karpv1.NodeClaimStatus{
			ProviderID: "digitalocean://nyc1/200",
		},
	}

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(nodeClaim).
		Build()

	controller := NewGarbageCollectionController(kubeClient, instanceProvider)

	_, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "claimed-node"},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// Orphan (100) should be deleted
	if _, ok := instanceProvider.Instances[100]; ok {
		t.Error("expected orphaned instance 100 to be deleted")
	}

	// Claimed instance (200) should still exist
	if _, ok := instanceProvider.Instances[200]; !ok {
		t.Error("expected claimed instance 200 to still exist")
	}
}

func TestGCNoOrphanedDroplets(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()

	// Add instance with a matching NodeClaim
	instanceProvider.Instances[100] = &instance.Instance{
		ID:     100,
		Name:   "node-1",
		Region: "nyc1",
		Size:   "s-1vcpu-1gb",
		Status: "active",
	}

	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Status: karpv1.NodeClaimStatus{
			ProviderID: "digitalocean://nyc1/100",
		},
	}

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(nodeClaim).
		Build()

	controller := NewGarbageCollectionController(kubeClient, instanceProvider)

	_, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "node-1"},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// Instance should still exist
	if _, ok := instanceProvider.Instances[100]; !ok {
		t.Error("expected instance 100 to still exist (not orphaned)")
	}
}

func TestGCNoInstances(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	controller := NewGarbageCollectionController(kubeClient, instanceProvider)

	result, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "any"},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue")
	}
}

func TestGCListInstancesError(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()
	instanceProvider.ListError = fmt.Errorf("API error")

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	controller := NewGarbageCollectionController(kubeClient, instanceProvider)

	_, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "any"},
	})
	if err == nil {
		t.Fatal("expected error when instance listing fails")
	}
}

func TestGCDeleteOrphanFails(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()

	// Add an orphaned instance
	instanceProvider.Instances[100] = &instance.Instance{
		ID:     100,
		Name:   "orphan-node",
		Region: "nyc1",
		Size:   "s-1vcpu-1gb",
		Status: "active",
	}

	// Set delete error so deletion fails
	instanceProvider.DeleteError = fmt.Errorf("delete failed")

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	controller := NewGarbageCollectionController(kubeClient, instanceProvider)

	// Should not return an error — we log and continue
	_, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "any"},
	})
	if err != nil {
		t.Fatalf("Reconcile() should not return error on individual delete failure, got: %v", err)
	}

	// The instance should still exist since delete failed
	if _, ok := instanceProvider.Instances[100]; !ok {
		t.Error("expected instance 100 to still exist since delete failed")
	}
}

func TestGCMultipleOrphans(t *testing.T) {
	ctx := context.Background()

	instanceProvider := fakeproviders.NewInstanceProvider()

	// Add multiple orphaned instances
	for i := 100; i < 105; i++ {
		instanceProvider.Instances[i] = &instance.Instance{
			ID:     i,
			Name:   fmt.Sprintf("orphan-%d", i),
			Region: "nyc1",
			Size:   "s-1vcpu-1gb",
			Status: "active",
		}
	}

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	controller := NewGarbageCollectionController(kubeClient, instanceProvider)

	_, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "any"},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// All orphans should be deleted
	if len(instanceProvider.Instances) != 0 {
		t.Errorf("expected all orphans to be deleted, got %d remaining", len(instanceProvider.Instances))
	}
}
