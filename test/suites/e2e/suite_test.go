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
//   - DO_REGION: the region the cluster is in (default: nyc1)
//   - DO_VPC_UUID: the VPC UUID of the cluster
//   - CLUSTER_NAME: the cluster name
//   - CLUSTER_ENDPOINT: the cluster API endpoint
package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Global test state shared across all e2e tests.
var (
	kubeClient    kubernetes.Interface
	dynamicClient dynamic.Interface
	restConfig    *rest.Config

	testRegion          string
	testVPCUUID         string
	testClusterName     string
	testClusterID       string
	testClusterEndpoint string
)

func TestMain(m *testing.M) {
	if os.Getenv("DIGITALOCEAN_ACCESS_TOKEN") == "" {
		fmt.Println("DIGITALOCEAN_ACCESS_TOKEN not set, skipping e2e tests")
		os.Exit(0)
	}
	if os.Getenv("DO_CLUSTER_ID") == "" {
		fmt.Println("DO_CLUSTER_ID not set, skipping e2e tests")
		os.Exit(0)
	}

	var err error

	// Load kubeconfig — try KUBECONFIG env, then default path.
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Printf("Failed to load kubeconfig from %s: %v\n", kubeconfig, err)
		os.Exit(1)
	}

	kubeClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		fmt.Printf("Failed to create Kubernetes clientset: %v\n", err)
		os.Exit(1)
	}

	dynamicClient, err = dynamic.NewForConfig(restConfig)
	if err != nil {
		fmt.Printf("Failed to create dynamic client: %v\n", err)
		os.Exit(1)
	}

	// Populate environment.
	testRegion = os.Getenv("DO_REGION")
	if testRegion == "" {
		testRegion = "nyc1"
	}
	testVPCUUID = os.Getenv("DO_VPC_UUID")
	testClusterName = os.Getenv("CLUSTER_NAME")
	testClusterID = os.Getenv("DO_CLUSTER_ID")
	testClusterEndpoint = os.Getenv("CLUSTER_ENDPOINT")

	os.Exit(m.Run())
}
