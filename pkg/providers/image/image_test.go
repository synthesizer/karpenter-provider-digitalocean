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

package image

import (
	"context"
	"testing"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	fakeproviders "github.com/digitalocean/karpenter-provider-digitalocean/pkg/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestResolveWithDirectID verifies that a direct image ID is returned immediately.
func TestResolveWithDirectID(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewImageProvider()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			Image:  v1alpha1.DONodeClassImage{ID: 55555},
		},
	}

	id, err := provider.Resolve(ctx, nodeClass)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if id != 55555 {
		t.Errorf("Resolve() = %d, want %d", id, 55555)
	}
}

// TestResolveWithSlug verifies slug resolution falls back to default ID.
func TestResolveWithSlug(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewImageProvider()
	provider.DefaultImageID = 99999

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			Image:  v1alpha1.DONodeClassImage{Slug: "ubuntu-24-04-x64"},
		},
	}

	id, err := provider.Resolve(ctx, nodeClass)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if id != 99999 {
		t.Errorf("Resolve() = %d, want %d", id, 99999)
	}
}

// TestResolveWithMappedSlug verifies a specific slug mapping.
func TestResolveWithMappedSlug(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewImageProvider()
	provider.ImageIDs["custom-image"] = 77777

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			Image:  v1alpha1.DONodeClassImage{Slug: "custom-image"},
		},
	}

	id, err := provider.Resolve(ctx, nodeClass)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if id != 77777 {
		t.Errorf("Resolve() = %d, want %d", id, 77777)
	}
}

// TestResolveWithNoImageSpec verifies error on missing image specification.
func TestResolveWithNoImageSpec(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewImageProvider()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			// No image ID or slug
		},
	}

	_, err := provider.Resolve(ctx, nodeClass)
	if err == nil {
		t.Fatal("expected error when no image is specified")
	}
}

// TestResolveError verifies error propagation from the provider.
func TestResolveError(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewImageProvider()
	provider.ResolveError = context.DeadlineExceeded

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			Image:  v1alpha1.DONodeClassImage{Slug: "ubuntu-24-04-x64"},
		},
	}

	_, err := provider.Resolve(ctx, nodeClass)
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

// TestResolveIDTakesPrecedenceOverSlug verifies that ID takes priority.
func TestResolveIDTakesPrecedenceOverSlug(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewImageProvider()
	provider.DefaultImageID = 99999

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			Image: v1alpha1.DONodeClassImage{
				ID:   11111,
				Slug: "ubuntu-24-04-x64",
			},
		},
	}

	id, err := provider.Resolve(ctx, nodeClass)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if id != 11111 {
		t.Errorf("Resolve() = %d, want %d (ID should take precedence over slug)", id, 11111)
	}
}

// TestResolveCallCount verifies that the resolve call is tracked.
func TestResolveCallCount(t *testing.T) {
	ctx := context.Background()
	provider := fakeproviders.NewImageProvider()

	nodeClass := &v1alpha1.DONodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
		Spec: v1alpha1.DONodeClassSpec{
			Region: "nyc1",
			Image:  v1alpha1.DONodeClassImage{ID: 12345},
		},
	}

	for i := 0; i < 5; i++ {
		_, _ = provider.Resolve(ctx, nodeClass)
	}

	if provider.ResolveCalls != 5 {
		t.Errorf("expected 5 resolve calls, got %d", provider.ResolveCalls)
	}
}
