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

package integration

import (
	"testing"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/image"
)

func TestImageProvider_ResolveBySlug(t *testing.T) {
	provider := image.NewDefaultProvider(env.client)

	nodeClass := &v1alpha1.DONodeClass{}
	nodeClass.Spec.Image.Slug = "ubuntu-24-04-x64"

	imageID, err := provider.Resolve(env.ctx, nodeClass)
	if err != nil {
		t.Fatalf("Resolve(slug=ubuntu-24-04-x64) error: %v", err)
	}

	if imageID == 0 {
		t.Error("Resolve returned image ID 0")
	}

	t.Logf("ubuntu-24-04-x64 resolved to image ID %d", imageID)
}

func TestImageProvider_ResolveByID(t *testing.T) {
	provider := image.NewDefaultProvider(env.client)

	// First resolve a slug to get a valid image ID.
	lookup := &v1alpha1.DONodeClass{}
	lookup.Spec.Image.Slug = "ubuntu-24-04-x64"

	knownID, err := provider.Resolve(env.ctx, lookup)
	if err != nil {
		t.Fatalf("Resolve(slug) error: %v", err)
	}

	// Now resolve by that ID directly.
	nodeClass := &v1alpha1.DONodeClass{}
	nodeClass.Spec.Image.ID = knownID

	imageID, err := provider.Resolve(env.ctx, nodeClass)
	if err != nil {
		t.Fatalf("Resolve(id=%d) error: %v", knownID, err)
	}

	if imageID != knownID {
		t.Errorf("Resolve(id=%d) = %d, want %d", knownID, imageID, knownID)
	}
}

func TestImageProvider_ResolveInvalidSlug(t *testing.T) {
	provider := image.NewDefaultProvider(env.client)

	nodeClass := &v1alpha1.DONodeClass{}
	nodeClass.Spec.Image.Slug = "nonexistent-image-slug-xyz"

	_, err := provider.Resolve(env.ctx, nodeClass)
	if err == nil {
		t.Error("Resolve(nonexistent slug) should return error")
	}
	t.Logf("Expected error for invalid slug: %v", err)
}

func TestImageProvider_ResolveNoImageSpec(t *testing.T) {
	provider := image.NewDefaultProvider(env.client)

	nodeClass := &v1alpha1.DONodeClass{}
	nodeClass.Name = "empty-image-spec"

	_, err := provider.Resolve(env.ctx, nodeClass)
	if err == nil {
		t.Error("Resolve(empty spec) should return error")
	}
	t.Logf("Expected error for empty spec: %v", err)
}
