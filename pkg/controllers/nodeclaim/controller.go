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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instance"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// GarbageCollectionController reconciles NodeClaims to clean up orphaned droplets.
type GarbageCollectionController struct {
	client           client.Client
	instanceProvider instance.Provider
}

// NewGarbageCollectionController creates a new garbage collection controller.
func NewGarbageCollectionController(client client.Client, instanceProvider instance.Provider) *GarbageCollectionController {
	return &GarbageCollectionController{
		client:           client,
		instanceProvider: instanceProvider,
	}
}

// Reconcile handles orphaned droplet cleanup.
// It lists all karpenter-managed droplets and checks if each has a corresponding NodeClaim.
// Droplets without a NodeClaim are considered orphaned and are deleted.
func (c *GarbageCollectionController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("running garbage collection reconciliation")

	// List all managed instances
	instances, err := c.instanceProvider.List(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("listing instances: %w", err)
	}

	// List all NodeClaims
	nodeClaimList := &karpv1.NodeClaimList{}
	if err := c.client.List(ctx, nodeClaimList); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing NodeClaims: %w", err)
	}

	// Build set of known provider IDs
	knownIDs := make(map[string]bool)
	for _, nc := range nodeClaimList.Items {
		if nc.Status.ProviderID != "" {
			knownIDs[nc.Status.ProviderID] = true
		}
	}

	// Find and delete orphaned instances
	for _, inst := range instances {
		providerID := fmt.Sprintf("digitalocean://%s/%d", inst.Region, inst.ID)
		if !knownIDs[providerID] {
			logger.Info("deleting orphaned droplet", "id", inst.ID, "name", inst.Name)
			if err := c.instanceProvider.Delete(ctx, fmt.Sprintf("%d", inst.ID)); err != nil {
				logger.Error(err, "failed to delete orphaned droplet", "id", inst.ID)
			}
		}
	}

	return ctrl.Result{}, nil
}

// Register implements operatorpkg/controller.Controller, allowing this controller
// to be registered with the Karpenter core operator's controller registration system.
func (c *GarbageCollectionController) Register(_ context.Context, mgr manager.Manager) error {
	return c.SetupWithManager(mgr)
}

// SetupWithManager sets up the controller with the Manager.
func (c *GarbageCollectionController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karpv1.NodeClaim{}).
		Complete(c)
}
