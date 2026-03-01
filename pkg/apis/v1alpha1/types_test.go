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
	"testing"

	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetConditions(t *testing.T) {
	nc := &DONodeClass{
		Status: DONodeClassStatus{
			Conditions: []status.Condition{
				{
					Type:   ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				},
			},
		},
	}

	conditions := nc.GetConditions()
	if len(conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(conditions))
	}
	if conditions[0].Type != ConditionTypeReady {
		t.Errorf("expected condition type %q, got %q", ConditionTypeReady, conditions[0].Type)
	}
}

func TestSetConditions(t *testing.T) {
	nc := &DONodeClass{}

	conditions := []status.Condition{
		{
			Type:   ConditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		},
		{
			Type:   ConditionTypeImageResolved,
			Status: metav1.ConditionTrue,
			Reason: "Resolved",
		},
	}

	nc.SetConditions(conditions)

	got := nc.GetConditions()
	if len(got) != 2 {
		t.Errorf("expected 2 conditions, got %d", len(got))
	}
}

func TestStatusConditions(t *testing.T) {
	nc := &DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Status: DONodeClassStatus{},
	}

	cs := nc.StatusConditions()
	// StatusConditions() should return a ConditionSet that initializes conditions
	root := cs.Root()
	if root == nil {
		t.Fatal("StatusConditions().Root() returned nil")
	}
	// The root condition (Ready) should be set to Unknown by default
	if root.Status != metav1.ConditionUnknown {
		t.Errorf("expected root condition status %q, got %q", metav1.ConditionUnknown, root.Status)
	}
}

func TestDONodeClassDeepCopy(t *testing.T) {
	userData := "#!/bin/bash\necho hello"
	nc := &DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: DONodeClassSpec{
			Region:  "nyc1",
			VPCUUID: "vpc-123",
			Image: DONodeClassImage{
				Slug: "ubuntu-24-04-x64",
			},
			SSHKeys:  []string{"fp1", "fp2"},
			Tags:     []string{"env:test"},
			UserData: &userData,
			BlockStorage: &DOBlockStorageSpec{
				SizeGiB: 100,
				FSType:  "ext4",
			},
		},
		Status: DONodeClassStatus{
			ImageID: 12345,
			Conditions: []status.Condition{
				{
					Type:   ConditionTypeReady,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	copied := nc.DeepCopy()

	// Verify the copy is independent
	if copied.Name != nc.Name {
		t.Errorf("DeepCopy name = %q, want %q", copied.Name, nc.Name)
	}
	if copied.Spec.Region != nc.Spec.Region {
		t.Errorf("DeepCopy region = %q, want %q", copied.Spec.Region, nc.Spec.Region)
	}

	// Modify the copy and verify the original is not affected
	copied.Spec.Region = "sfo3"
	if nc.Spec.Region == "sfo3" {
		t.Error("modifying copy affected original — DeepCopy is not deep")
	}

	// Verify slice independence
	copied.Spec.SSHKeys[0] = "modified"
	if nc.Spec.SSHKeys[0] == "modified" {
		t.Error("modifying copy's SSHKeys affected original")
	}

	// Verify pointer independence
	*copied.Spec.UserData = "changed"
	if *nc.Spec.UserData == "changed" {
		t.Error("modifying copy's UserData affected original")
	}
}
