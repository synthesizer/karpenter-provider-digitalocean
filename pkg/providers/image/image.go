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
	"fmt"

	"github.com/digitalocean/godo"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
)

// Provider resolves OS images for DigitalOcean Droplets.
type Provider interface {
	// Resolve returns the image ID for the given DONodeClass.
	Resolve(ctx context.Context, nodeClass *v1alpha1.DONodeClass) (int, error)
}

// DefaultProvider implements the image Provider using the DigitalOcean API.
type DefaultProvider struct {
	doClient *godo.Client
}

// NewDefaultProvider creates a new image provider.
func NewDefaultProvider(doClient *godo.Client) *DefaultProvider {
	return &DefaultProvider{
		doClient: doClient,
	}
}

// Resolve returns the image ID for the given DONodeClass configuration.
// If an explicit image ID is set, it is returned directly.
// If a slug is set, it is resolved to an image ID via the DO API.
func (p *DefaultProvider) Resolve(ctx context.Context, nodeClass *v1alpha1.DONodeClass) (int, error) {
	// Direct image ID takes precedence
	if nodeClass.Spec.Image.ID != 0 {
		return nodeClass.Spec.Image.ID, nil
	}

	// Resolve slug to image ID
	if nodeClass.Spec.Image.Slug != "" {
		image, _, err := p.doClient.Images.GetBySlug(ctx, nodeClass.Spec.Image.Slug)
		if err != nil {
			return 0, fmt.Errorf("resolving image slug %q: %w", nodeClass.Spec.Image.Slug, err)
		}
		return image.ID, nil
	}

	return 0, fmt.Errorf("no image slug or ID specified in DONodeClass %q", nodeClass.Name)
}
