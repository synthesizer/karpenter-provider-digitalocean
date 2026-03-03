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

// Package integration contains integration tests that exercise the DigitalOcean
// provider packages against the live DigitalOcean API.
//
// These tests require the following environment variables:
//
//   - DIGITALOCEAN_ACCESS_TOKEN: a valid DigitalOcean API token
//   - DO_REGION: the region to test in (e.g. "nyc1")
//   - DO_VPC_UUID: (optional) the VPC UUID to use; if empty, the default VPC is resolved
//   - CLUSTER_NAME: (optional) name used for tagging; defaults to "integration-test"
package integration

import (
	"context"
	"os"
	"testing"

	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
)

// testEnv holds shared state for the integration test suite.
type testEnv struct {
	token       string
	region      string
	vpcUUID     string
	clusterName string
	client      *godo.Client
	ctx         context.Context
}

var env *testEnv

// TestMain sets up the test environment for the integration suite.
func TestMain(m *testing.M) {
	token := os.Getenv("DIGITALOCEAN_ACCESS_TOKEN")
	if token == "" {
		// Skip the entire suite when no token is available (e.g. local dev without creds).
		os.Exit(0)
	}

	region := os.Getenv("DO_REGION")
	if region == "" {
		region = "nyc1"
	}

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "integration-test"
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	client := godo.NewClient(oauth2.NewClient(context.Background(), ts))

	env = &testEnv{
		token:       token,
		region:      region,
		vpcUUID:     os.Getenv("DO_VPC_UUID"),
		clusterName: clusterName,
		client:      client,
		ctx:         context.Background(),
	}

	os.Exit(m.Run())
}
