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

package instancetype

import (
	"testing"
)

func TestExtractFamily(t *testing.T) {
	tests := []struct {
		slug     string
		expected string
	}{
		{"s-1vcpu-1gb", "s"},
		{"s-1vcpu-2gb", "s"},
		{"s-2vcpu-4gb", "s"},
		{"s-4vcpu-8gb", "s"},
		{"s-8vcpu-16gb", "s"},
		{"g-2vcpu-8gb", "g"},
		{"g-4vcpu-16gb", "g"},
		{"g-8vcpu-32gb", "g"},
		{"c-2vcpu-4gb", "c"},
		{"c-4vcpu-8gb", "c"},
		{"m-2vcpu-16gb", "m"},
		{"m-4vcpu-32gb", "m"},
		{"so-2vcpu-16gb", "so"},
		{"so-4vcpu-32gb", "so"},
		{"gd-2vcpu-8gb", "gd"},
		{"gd-4vcpu-16gb", "gd"},
		{"gpu-h100x1-80gb", "gpu"},
		{"gpu-h100x8-640gb", "gpu"},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			got := extractFamily(tt.slug)
			if got != tt.expected {
				t.Errorf("extractFamily(%q) = %q, want %q", tt.slug, got, tt.expected)
			}
		})
	}
}
