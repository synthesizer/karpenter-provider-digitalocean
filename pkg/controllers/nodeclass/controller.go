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
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/image"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/vpc"
)

// Controller reconciles DONodeClass resources.
type Controller struct {
	client        client.Client
	imageProvider image.Provider
	vpcProvider   vpc.Provider
}

// NewController creates a new NodeClass controller.
func NewController(client client.Client, imageProvider image.Provider, vpcProvider vpc.Provider) *Controller {
	return &Controller{
		client:        client,
		imageProvider: imageProvider,
		vpcProvider:   vpcProvider,
	}
}

// Reconcile handles DONodeClass reconciliation.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling DONodeClass", "name", req.Name)

	// Fetch the DONodeClass
	nodeClass := &v1alpha1.DONodeClass{}
	if err := c.client.Get(ctx, req.NamespacedName, nodeClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve image
	imageID, err := c.imageProvider.Resolve(ctx, nodeClass)
	if err != nil {
		c.setCondition(nodeClass, v1alpha1.ConditionTypeImageResolved, metav1.ConditionFalse, "ResolveFailed", err.Error())
		if updateErr := c.client.Status().Update(ctx, nodeClass); updateErr != nil {
			return ctrl.Result{}, fmt.Errorf("updating status: %w", updateErr)
		}
		return ctrl.Result{}, fmt.Errorf("resolving image: %w", err)
	}

	nodeClass.Status.ImageID = imageID
	c.setCondition(nodeClass, v1alpha1.ConditionTypeImageResolved, metav1.ConditionTrue, "Resolved", fmt.Sprintf("Image resolved to ID %d", imageID))

	// Validate VPC if specified
	if nodeClass.Spec.VPCUUID != "" {
		if _, err := c.vpcProvider.Get(ctx, nodeClass.Spec.VPCUUID); err != nil {
			c.setCondition(nodeClass, v1alpha1.ConditionTypeVPCValid, metav1.ConditionFalse, "InvalidVPC", err.Error())
			if updateErr := c.client.Status().Update(ctx, nodeClass); updateErr != nil {
				return ctrl.Result{}, fmt.Errorf("updating status: %w", updateErr)
			}
			return ctrl.Result{}, fmt.Errorf("validating VPC: %w", err)
		}
		c.setCondition(nodeClass, v1alpha1.ConditionTypeVPCValid, metav1.ConditionTrue, "Valid", "VPC is valid and accessible")
	}

	// Set overall Ready condition
	c.setCondition(nodeClass, v1alpha1.ConditionTypeReady, metav1.ConditionTrue, "Ready", "DONodeClass is ready")

	// Update status
	if err := c.client.Status().Update(ctx, nodeClass); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	logger.Info("DONodeClass reconciled successfully", "name", req.Name, "imageID", imageID)
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
