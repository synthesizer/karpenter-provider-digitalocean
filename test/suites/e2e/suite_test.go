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

// Package e2e contains end-to-end tests that deploy Karpenter into a live
// DOKS cluster and exercise the full node provisioning lifecycle.
//
// Required environment variables:
//
//   - DIGITALOCEAN_ACCESS_TOKEN: a valid DigitalOcean API token
//   - DO_CLUSTER_ID: the DOKS cluster ID
//   - DO_REGION: the region the cluster is in
//   - DO_VPC_UUID: the VPC UUID of the cluster
//   - CLUSTER_NAME: the cluster name
package e2e

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Skip the entire suite when no token is available.
	if os.Getenv("DIGITALOCEAN_ACCESS_TOKEN") == "" {
		os.Exit(0)
	}
	if os.Getenv("DO_CLUSTER_ID") == "" {
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// TestPlaceholder ensures the e2e package compiles and the test runner finds
// at least one test. Replace this with real e2e tests as they are implemented.
func TestPlaceholder(t *testing.T) {
	t.Log("E2E test suite placeholder — real tests coming soon")
}
