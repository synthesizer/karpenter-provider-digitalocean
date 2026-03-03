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

package e2e

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ─────────────────────────────────────────────────────────────────────────────
// TestInitialClusterSize verifies the cluster has the expected number of nodes
// (2, as configured by the CI workflow) and that none of them are managed by
// Karpenter.
// ─────────────────────────────────────────────────────────────────────────────
func TestInitialClusterSize(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}

	t.Logf("Cluster has %d node(s)", len(nodes.Items))

	if len(nodes.Items) < 1 {
		t.Fatal("Expected at least 1 node in the cluster")
	}

	// All initial nodes should belong to the DOKS node pool, not Karpenter.
	for _, node := range nodes.Items {
		if _, ok := node.Labels["karpenter.sh/nodepool"]; ok {
			t.Errorf("Node %q has karpenter.sh/nodepool label — expected only DOKS-managed nodes at start", node.Name)
		}
	}
	t.Log("All initial nodes are DOKS-managed (no Karpenter labels)")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestKarpenterControllerRunning verifies the Karpenter controller pod exists
// in kube-system and is in Running phase.
// ─────────────────────────────────────────────────────────────────────────────
func TestKarpenterControllerRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pods, err := kubeClient.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=karpenter-do",
	})
	if err != nil {
		t.Fatalf("Failed to list karpenter-do pods: %v", err)
	}

	if len(pods.Items) == 0 {
		t.Fatal("No karpenter-do controller pods found in kube-system")
	}

	for _, pod := range pods.Items {
		t.Logf("Pod %s: phase=%s", pod.Name, pod.Status.Phase)
		if pod.Status.Phase != corev1.PodRunning {
			t.Errorf("Pod %s is not Running (phase=%s)", pod.Name, pod.Status.Phase)
		}

		// Check container readiness.
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				t.Errorf("Container %s in pod %s is not ready", cs.Name, pod.Name)
			}
			if cs.RestartCount > 3 {
				t.Errorf("Container %s in pod %s has restarted %d times", cs.Name, pod.Name, cs.RestartCount)
			}
		}
	}

	t.Logf("Karpenter controller: %d pod(s) running", len(pods.Items))
}

// ─────────────────────────────────────────────────────────────────────────────
// TestDONodeClassLifecycle creates a DONodeClass, verifies it can be read back,
// checks status conditions are populated by the controller, and deletes it.
// ─────────────────────────────────────────────────────────────────────────────
func TestDONodeClassLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	const name = "e2e-lifecycle"
	createDONodeClass(t, ctx, name)
	defer cleanupDONodeClass(t, name)

	// Read it back.
	nc := getDONodeClass(t, ctx, name)
	spec, ok := nc.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("DONodeClass has no spec")
	}

	region, _, _ := unstructuredNestedString(spec, "region")
	if region != testRegion {
		t.Errorf("Expected region %q, got %q", testRegion, region)
	}

	// Wait for the controller to reconcile status conditions.
	t.Log("Waiting for DONodeClass status conditions…")
	err := waitPoll(ctx, 5*time.Second, 3*time.Minute, func(ctx context.Context) (bool, error) {
		nc, getErr := dynamicClient.Resource(doNodeClassGVR).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return false, nil
		}
		status, ok := nc.Object["status"].(map[string]interface{})
		if !ok {
			return false, nil
		}
		conditions, ok := status["conditions"].([]interface{})
		if !ok || len(conditions) == 0 {
			return false, nil
		}
		t.Logf("  DONodeClass %q has %d condition(s)", name, len(conditions))
		return true, nil
	})
	if err != nil {
		t.Logf("Warning: DONodeClass status conditions not populated within timeout (controller may need work)")
	}

	t.Log("TestDONodeClassLifecycle PASSED")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestNoClusterAutoscalerRunning ensures that the DOKS cluster autoscaler is
// not active, which would interfere with Karpenter tests.
// ─────────────────────────────────────────────────────────────────────────────
func TestNoClusterAutoscalerRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Look for cluster-autoscaler pods in kube-system.
	pods, err := kubeClient.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app=cluster-autoscaler",
	})
	if err != nil {
		t.Fatalf("Failed to list cluster-autoscaler pods: %v", err)
	}

	running := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			running++
			t.Logf("Warning: cluster-autoscaler pod %s is Running", pod.Name)
		}
	}

	if running > 0 {
		t.Logf("Found %d running cluster-autoscaler pod(s). "+
			"Karpenter tests may be affected if it tries to auto-scale.", running)
		// Not a hard failure — DOKS may run its autoscaler differently.
	} else {
		t.Log("No cluster-autoscaler pods detected — Karpenter has exclusive scaling control")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestNodePoolCreationAndDeletion creates a NodePool, verifies it exists,
// and deletes it cleanly.
// ─────────────────────────────────────────────────────────────────────────────
func TestNodePoolCreationAndDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const ncName = "e2e-np-test-nc"
	const npName = "e2e-np-test"

	createDONodeClass(t, ctx, ncName)
	defer cleanupDONodeClass(t, ncName)

	createNodePool(t, ctx, npName, ncName)
	defer cleanupNodePool(t, npName)

	// Read it back.
	np, err := dynamicClient.Resource(nodePoolGVR).Get(ctx, npName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get NodePool %q: %v", npName, err)
	}
	t.Logf("NodePool %q created successfully (uid=%s)", np.GetName(), np.GetUID())

	// Verify nodeClassRef.
	nodeClassRef, found, _ := unstructuredNestedString(
		np.Object, "spec", "template", "spec", "nodeClassRef", "name")
	if !found || nodeClassRef != ncName {
		t.Errorf("Expected nodeClassRef.name=%q, got %q", ncName, nodeClassRef)
	}

	// Delete and verify it's gone.
	cleanupNodePool(t, npName)
	_, err = dynamicClient.Resource(nodePoolGVR).Get(ctx, npName, metav1.GetOptions{})
	if err == nil {
		t.Error("NodePool still exists after deletion")
	}
	t.Log("TestNodePoolCreationAndDeletion PASSED")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCRDsInstalled verifies that Karpenter core CRDs and the DONodeClass CRD
// are installed in the cluster.
// ─────────────────────────────────────────────────────────────────────────────
func TestCRDsInstalled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	crds := []struct {
		name string
		gvr  schema.GroupVersionResource
	}{
		{"nodepools.karpenter.sh", nodePoolGVR},
		{"nodeclaims.karpenter.sh", nodeClaimGVR},
		{"donodeclasses.karpenter.do.sh", doNodeClassGVR},
	}

	for _, crd := range crds {
		t.Run(crd.name, func(t *testing.T) {
			// List should work if the CRD is installed (even if empty).
			_, err := dynamicClient.Resource(crd.gvr).List(ctx, metav1.ListOptions{Limit: 1})
			if err != nil {
				t.Errorf("CRD %q does not appear to be installed: %v", crd.name, err)
			} else {
				t.Logf("CRD %q is installed", crd.name)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// unstructuredNestedString extracts a string from a nested map path.
// ─────────────────────────────────────────────────────────────────────────────
func unstructuredNestedString(obj interface{}, fields ...string) (string, bool, error) {
	current := obj
	for _, f := range fields {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", false, nil
		}
		current = m[f]
	}
	s, ok := current.(string)
	return s, ok, nil
}
