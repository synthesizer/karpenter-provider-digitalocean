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
type DONodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DONodeClassSpec   `json:"spec,omitempty"`
	Status DONodeClassStatus `json:"status,omitempty"`
}

// DONodeClassSpec defines the desired state of DONodeClass.
type DONodeClassSpec struct {
	// Region is the DigitalOcean region where droplets will be created.
	// Examples: "nyc1", "sfo3", "ams3", "sgp1"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=3
	Region string `json:"region"`

	// VPCUUID is the UUID of the VPC where droplets will be placed.
	// If empty, the default VPC for the region will be used.
	// +optional
	VPCUUID string `json:"vpcUUID,omitempty"`

	// Image specifies the OS image configuration for provisioned droplets.
	// +kubebuilder:validation:Required
	Image DONodeClassImage `json:"image"`

	// SSHKeys is a list of SSH key fingerprints or IDs to add to provisioned droplets.
	// +optional
	SSHKeys []string `json:"sshKeys,omitempty"`

	// Tags are additional tags applied to all droplets managed by this DONodeClass.
	// Karpenter-managed tags are automatically added and should not be specified here.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// UserData is a cloud-init script or bash script to execute on droplet creation.
	// This is used to bootstrap the node and join it to the Kubernetes cluster.
	// +optional
	UserData *string `json:"userData,omitempty"`

	// BlockStorage configures block storage volumes to attach to provisioned droplets.
	// +optional
	BlockStorage *DOBlockStorageSpec `json:"blockStorage,omitempty"`
}

// DONodeClassImage specifies the image to use for provisioned droplets.
type DONodeClassImage struct {
	// Slug is a well-known DigitalOcean image slug (e.g., "ubuntu-24-04-x64").
	// Mutually exclusive with ID.
	// +optional
	Slug string `json:"slug,omitempty"`

	// ID is a custom image or snapshot ID.
	// Mutually exclusive with Slug.
	// +optional
	ID int `json:"id,omitempty"`
}

// DOBlockStorageSpec configures block storage volumes for droplets.
type DOBlockStorageSpec struct {
	// SizeGiB is the size of the block storage volume in GiB.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16384
	SizeGiB int `json:"sizeGiB"`

	// FSType is the filesystem type for the volume (e.g., "ext4", "xfs").
	// +kubebuilder:default="ext4"
	// +optional
	FSType string `json:"fsType,omitempty"`
}

// DONodeClassStatus defines the observed state of DONodeClass.
type DONodeClassStatus struct {
	// ImageID is the resolved DigitalOcean image ID.
	// This is populated after the image slug is resolved to a concrete ID.
	// +optional
	ImageID int `json:"imageID,omitempty"`

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

	// ConditionTypeImageResolved indicates the image has been resolved.
	ConditionTypeImageResolved = "ImageResolved"

	// ConditionTypeVPCValid indicates the VPC is valid and accessible.
	ConditionTypeVPCValid = "VPCValid"
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
		ConditionTypeImageResolved,
		ConditionTypeVPCValid,
	).For(in)
}
