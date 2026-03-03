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

package nodeclass

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	fakeproviders "github.com/digitalocean/karpenter-provider-digitalocean/pkg/fake"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func TestReconcileValidRegion(t *testing.T) {
	ctx := context.Background()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
		},
	}

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(nodeClass).
		WithStatusSubresource(&v1alpha1.DONodeClass{}).
		Build()

	regionProvider := fakeproviders.NewRegionProvider()
	controller := NewController(kubeClient, regionProvider, "nyc1")

	result, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-class"},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue")
	}

	// Verify conditions
	updated := &v1alpha1.DONodeClass{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updated); err != nil {
		t.Fatalf("failed to get updated DONodeClass: %v", err)
	}
	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected conditions to be set")
	}

	foundValidRegion := false
	foundReady := false
	for _, c := range updated.Status.Conditions {
		if c.Type == v1alpha1.ConditionTypeValidRegion && c.Status == metav1.ConditionTrue {
			foundValidRegion = true
		}
		if c.Type == v1alpha1.ConditionTypeReady && c.Status == metav1.ConditionTrue {
			foundReady = true
		}
	}
	if !foundValidRegion {
		t.Error("expected ValidRegion condition to be True")
	}
	if !foundReady {
		t.Error("expected Ready condition to be True")
	}
}

func TestReconcileRegionNotAvailable(t *testing.T) {
	ctx := context.Background()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nonexistent-region",
		},
	}

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(nodeClass).
		WithStatusSubresource(&v1alpha1.DONodeClass{}).
		Build()

	regionProvider := fakeproviders.NewRegionProvider()
	controller := NewController(kubeClient, regionProvider, "nyc1")

	_, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-class"},
	})
	if err == nil {
		t.Fatal("expected error from unavailable region")
	}

	// Verify the failed condition was set
	updated := &v1alpha1.DONodeClass{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updated); err != nil {
		t.Fatalf("failed to get updated DONodeClass: %v", err)
	}

	for _, c := range updated.Status.Conditions {
		if c.Type == v1alpha1.ConditionTypeValidRegion && c.Status == metav1.ConditionFalse {
			return // success
		}
	}
	t.Error("expected ValidRegion condition to be False")
}

func TestReconcileRegionMismatch(t *testing.T) {
	ctx := context.Background()

	// DONodeClass region doesn't match cluster region
	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "sfo3",
		},
	}

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(nodeClass).
		WithStatusSubresource(&v1alpha1.DONodeClass{}).
		Build()

	regionProvider := fakeproviders.NewRegionProvider()
	controller := NewController(kubeClient, regionProvider, "nyc1") // cluster is in nyc1, DONodeClass says sfo3

	_, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-class"},
	})
	if err == nil {
		t.Fatal("expected error from region mismatch")
	}

	// Verify the failed condition was set
	updated := &v1alpha1.DONodeClass{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updated); err != nil {
		t.Fatalf("failed to get updated DONodeClass: %v", err)
	}

	for _, c := range updated.Status.Conditions {
		if c.Type == v1alpha1.ConditionTypeValidRegion && c.Status == metav1.ConditionFalse {
			if c.Reason == "RegionMismatch" {
				return // success
			}
		}
	}
	t.Error("expected ValidRegion condition to be False with RegionMismatch reason")
}

func TestReconcileNotFound(t *testing.T) {
	ctx := context.Background()

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	regionProvider := fakeproviders.NewRegionProvider()
	controller := NewController(kubeClient, regionProvider, "nyc1")

	// Reconcile a non-existent DONodeClass — should return no error (IgnoreNotFound)
	result, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent"},
	})
	if err != nil {
		t.Fatalf("expected no error for not found, got: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue")
	}
}

func TestSetCondition(t *testing.T) {
	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}

	controller := &Controller{}

	// Set initial condition
	controller.setCondition(nodeClass, "TestCondition", metav1.ConditionTrue, "TestReason", "test message")

	if len(nodeClass.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(nodeClass.Status.Conditions))
	}
	if nodeClass.Status.Conditions[0].Type != "TestCondition" {
		t.Errorf("expected type TestCondition, got %s", nodeClass.Status.Conditions[0].Type)
	}
	if nodeClass.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", nodeClass.Status.Conditions[0].Status)
	}

	// Update same condition
	controller.setCondition(nodeClass, "TestCondition", metav1.ConditionFalse, "FailedReason", "failure")

	if len(nodeClass.Status.Conditions) != 1 {
		t.Fatalf("expected still 1 condition after update, got %d", len(nodeClass.Status.Conditions))
	}
	if nodeClass.Status.Conditions[0].Status != metav1.ConditionFalse {
		t.Errorf("expected updated status False, got %s", nodeClass.Status.Conditions[0].Status)
	}
	if nodeClass.Status.Conditions[0].Reason != "FailedReason" {
		t.Errorf("expected updated reason FailedReason, got %s", nodeClass.Status.Conditions[0].Reason)
	}

	// Add a different condition
	controller.setCondition(nodeClass, "AnotherCondition", metav1.ConditionTrue, "OK", "all good")

	if len(nodeClass.Status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(nodeClass.Status.Conditions))
	}
}

func TestSetConditionPreservesTransitionTime(t *testing.T) {
	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}

	controller := &Controller{}

	// Set initial condition
	controller.setCondition(nodeClass, "TestCondition", metav1.ConditionTrue, "OK", "ok")
	originalTime := nodeClass.Status.Conditions[0].LastTransitionTime

	// Update with same status — should preserve LastTransitionTime
	controller.setCondition(nodeClass, "TestCondition", metav1.ConditionTrue, "StillOK", "still ok")
	if !nodeClass.Status.Conditions[0].LastTransitionTime.Equal(&originalTime) {
		t.Error("LastTransitionTime should be preserved when status doesn't change")
	}
}

func TestReconcileWithTags(t *testing.T) {
	ctx := context.Background()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class-with-tags",
		},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			Tags:   []string{"env:prod", "team:platform"},
		},
	}

	s := newTestScheme()
	kubeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(nodeClass).
		WithStatusSubresource(&v1alpha1.DONodeClass{}).
		Build()

	regionProvider := fakeproviders.NewRegionProvider()
	controller := NewController(kubeClient, regionProvider, "nyc1")

	result, err := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-class-with-tags"},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue")
	}

	// Verify Ready condition
	updated := &v1alpha1.DONodeClass{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: "test-class-with-tags"}, updated); err != nil {
		t.Fatalf("failed to get updated DONodeClass: %v", err)
	}

	foundReady := false
	for _, c := range updated.Status.Conditions {
		if c.Type == v1alpha1.ConditionTypeReady && c.Status == metav1.ConditionTrue {
			foundReady = true
		}
	}
	if !foundReady {
		t.Error("expected Ready condition to be True")
	}
}
