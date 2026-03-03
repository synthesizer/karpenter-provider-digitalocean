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

package v1alpha1

import (
	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=karpenter,shortName=donc
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.region"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DONodeClass is the DigitalOcean-specific configuration for Karpenter node provisioning.
// It is referenced by a NodePool's spec.template.spec.nodeClassRef.
//
// When using DOKS (DigitalOcean Kubernetes Service), node images, VPC placement,
// and bootstrap configuration are managed automatically by DOKS. The DONodeClass
// primarily controls region placement and additional tags/metadata for node pools.
type DONodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DONodeClassSpec   `json:"spec,omitempty"`
	Status DONodeClassStatus `json:"status,omitempty"`
}

// DONodeClassSpec defines the desired state of DONodeClass.
// For DOKS clusters, most node configuration (images, VPC, bootstrap) is
// managed automatically. The spec focuses on region and metadata.
type DONodeClassSpec struct {
	// Region is the DigitalOcean region where DOKS node pools will be created.
	// This must match the region of the DOKS cluster.
	// Examples: "nyc1", "sfo3", "ams3", "sgp1"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=3
	Region string `json:"region"`

	// Tags are additional tags applied to all DOKS node pools managed by this DONodeClass.
	// Karpenter-managed tags are automatically added and should not be specified here.
	// +optional
	Tags []string `json:"tags,omitempty"`
}

// DONodeClassStatus defines the observed state of DONodeClass.
type DONodeClassStatus struct {
	// Conditions contains the conditions for the DONodeClass.
	// +optional
	Conditions []status.Condition `json:"conditions,omitempty"`

	// SpecHash is the hash of the DONodeClassSpec, used for drift detection.
	// +optional
	SpecHash string `json:"specHash,omitempty"`
}

// +kubebuilder:object:root=true

// DONodeClassList contains a list of DONodeClass resources.
type DONodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DONodeClass `json:"items"`
}

// DONodeClass condition types.
const (
	// ConditionTypeReady indicates the DONodeClass is ready for use.
	ConditionTypeReady = "Ready"

	// ConditionTypeValidRegion indicates the region is valid and matches the cluster.
	ConditionTypeValidRegion = "ValidRegion"
)

// GetConditions returns the conditions for the DONodeClass.
// This is required to implement the operatorpkg status.Object interface.
func (in *DONodeClass) GetConditions() []status.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions for the DONodeClass.
// This is required to implement the operatorpkg status.Object interface.
func (in *DONodeClass) SetConditions(conditions []status.Condition) {
	in.Status.Conditions = conditions
}

// StatusConditions returns a ConditionSet for the DONodeClass.
// This is required to implement the operatorpkg status.Object interface.
func (in *DONodeClass) StatusConditions() status.ConditionSet {
	return status.NewReadyConditions(
		ConditionTypeValidRegion,
	).For(in)
}
