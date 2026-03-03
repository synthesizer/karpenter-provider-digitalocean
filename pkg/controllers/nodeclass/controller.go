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
	"fmt"

	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/region"
)

// Controller reconciles DONodeClass resources.
// For DOKS, the controller validates that the DONodeClass region matches the
// cluster region, since DOKS node pools must be in the same region as the cluster.
type Controller struct {
	client         client.Client
	regionProvider region.Provider
	clusterRegion  string
}

// NewController creates a new NodeClass controller.
func NewController(client client.Client, regionProvider region.Provider, clusterRegion string) *Controller {
	return &Controller{
		client:         client,
		regionProvider: regionProvider,
		clusterRegion:  clusterRegion,
	}
}

// Reconcile handles DONodeClass reconciliation.
// It validates the region is valid and matches the cluster region.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling DONodeClass", "name", req.Name)

	// Fetch the DONodeClass
	nodeClass := &v1alpha1.DONodeClass{}
	if err := c.client.Get(ctx, req.NamespacedName, nodeClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Validate region exists and is available
	if !c.regionProvider.IsAvailable(ctx, nodeClass.Spec.Region) {
		c.setCondition(nodeClass, v1alpha1.ConditionTypeValidRegion, metav1.ConditionFalse,
			"InvalidRegion", fmt.Sprintf("Region %q is not available", nodeClass.Spec.Region))
		if updateErr := c.client.Status().Update(ctx, nodeClass); updateErr != nil {
			return ctrl.Result{}, fmt.Errorf("updating status: %w", updateErr)
		}
		return ctrl.Result{}, fmt.Errorf("region %q is not available", nodeClass.Spec.Region)
	}

	// Validate region matches the cluster region
	if nodeClass.Spec.Region != c.clusterRegion {
		c.setCondition(nodeClass, v1alpha1.ConditionTypeValidRegion, metav1.ConditionFalse,
			"RegionMismatch", fmt.Sprintf("DONodeClass region %q does not match cluster region %q", nodeClass.Spec.Region, c.clusterRegion))
		if updateErr := c.client.Status().Update(ctx, nodeClass); updateErr != nil {
			return ctrl.Result{}, fmt.Errorf("updating status: %w", updateErr)
		}
		return ctrl.Result{}, fmt.Errorf("DONodeClass region %q does not match cluster region %q", nodeClass.Spec.Region, c.clusterRegion)
	}

	c.setCondition(nodeClass, v1alpha1.ConditionTypeValidRegion, metav1.ConditionTrue,
		"Valid", fmt.Sprintf("Region %q is valid and matches cluster", nodeClass.Spec.Region))

	// Set overall Ready condition
	c.setCondition(nodeClass, v1alpha1.ConditionTypeReady, metav1.ConditionTrue, "Ready", "DONodeClass is ready")

	// Update status
	if err := c.client.Status().Update(ctx, nodeClass); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	logger.Info("DONodeClass reconciled successfully", "name", req.Name, "region", nodeClass.Spec.Region)
	return ctrl.Result{}, nil
}

// Register implements operatorpkg/controller.Controller, allowing this controller
// to be registered with the Karpenter core operator's controller registration system.
func (c *Controller) Register(_ context.Context, mgr manager.Manager) error {
	return c.SetupWithManager(mgr)
}

// SetupWithManager sets up the controller with the Manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DONodeClass{}).
		Complete(c)
}

// setCondition sets or updates a condition on the DONodeClass.
func (c *Controller) setCondition(nodeClass *v1alpha1.DONodeClass, condType string, condStatus metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := status.Condition{
		Type:               condType,
		Status:             condStatus,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	for i, existing := range nodeClass.Status.Conditions {
		if existing.Type == condType {
			if existing.Status != condStatus {
				condition.LastTransitionTime = now
			} else {
				condition.LastTransitionTime = existing.LastTransitionTime
			}
			nodeClass.Status.Conditions[i] = condition
			return
		}
	}
	nodeClass.Status.Conditions = append(nodeClass.Status.Conditions, condition)
}
