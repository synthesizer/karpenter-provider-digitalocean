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
			Type:   ConditionTypeValidRegion,
			Status: metav1.ConditionTrue,
			Reason: "Valid",
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
	nc := &DONodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: DONodeClassSpec{
			Region: "nyc1",
			Tags:   []string{"env:test", "team:platform"},
		},
		Status: DONodeClassStatus{
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
	if len(copied.Spec.Tags) != len(nc.Spec.Tags) {
		t.Errorf("DeepCopy tags length = %d, want %d", len(copied.Spec.Tags), len(nc.Spec.Tags))
	}

	// Modify the copy and verify the original is not affected
	copied.Spec.Region = "sfo3"
	if nc.Spec.Region == "sfo3" {
		t.Error("modifying copy affected original — DeepCopy is not deep")
	}

	// Verify slice independence (tags)
	copied.Spec.Tags[0] = "modified"
	if nc.Spec.Tags[0] == "modified" {
		t.Error("modifying copy's Tags affected original")
	}

	// Verify conditions slice independence
	copied.Status.Conditions[0].Status = metav1.ConditionFalse
	if nc.Status.Conditions[0].Status == metav1.ConditionFalse {
		t.Error("modifying copy's Conditions affected original")
	}
}

func TestDONodeClassListDeepCopy(t *testing.T) {
	list := &DONodeClassList{
		Items: []DONodeClass{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "class-1"},
				Spec:       DONodeClassSpec{Region: "nyc1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "class-2"},
				Spec:       DONodeClassSpec{Region: "sfo3", Tags: []string{"env:staging"}},
			},
		},
	}

	copied := list.DeepCopy()

	if len(copied.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(copied.Items))
	}

	// Modify the copy
	copied.Items[0].Spec.Region = "ams3"
	if list.Items[0].Spec.Region == "ams3" {
		t.Error("modifying copied list affected original")
	}
}

func TestDONodeClassSpecOnlyRegionAndTags(t *testing.T) {
	// Verify DONodeClassSpec structure — only Region and Tags should exist
	spec := DONodeClassSpec{
		Region: "nyc1",
		Tags:   []string{"karpenter", "env:prod"},
	}

	if spec.Region != "nyc1" {
		t.Errorf("expected region nyc1, got %s", spec.Region)
	}
	if len(spec.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(spec.Tags))
	}
}

func TestDONodeClassStatusNoImageID(t *testing.T) {
	// Verify DONodeClassStatus structure — no ImageID field, only Conditions and SpecHash
	s := DONodeClassStatus{
		SpecHash: "abc123",
		Conditions: []status.Condition{
			{
				Type:   ConditionTypeValidRegion,
				Status: metav1.ConditionTrue,
			},
		},
	}

	if s.SpecHash != "abc123" {
		t.Errorf("expected specHash abc123, got %s", s.SpecHash)
	}
	if len(s.Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(s.Conditions))
	}
}
