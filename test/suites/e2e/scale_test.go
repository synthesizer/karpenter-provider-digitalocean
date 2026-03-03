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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	defaultNamespace = "default"

	// Timeouts — droplet creation + boot can be slow.
	nodeClaimTimeout   = 5 * time.Minute
	nodeReadyTimeout   = 10 * time.Minute
	deployReadyTimeout = 12 * time.Minute
	scaleDownTimeout   = 10 * time.Minute
)

// ─────────────────────────────────────────────────────────────────────────────
// TestScaleUpAndDown verifies the complete Karpenter lifecycle:
//
//  1. Record initial cluster size.
//  2. Create DONodeClass + NodePool.
//  3. Deploy an "inflate" workload whose nodeSelector targets Karpenter nodes.
//  4. Karpenter sees the pending pod, creates a NodeClaim, and provisions a
//     DigitalOcean Droplet.
//  5. The Droplet joins the cluster, the pod is scheduled and becomes Running.
//  6. Verify the new node was created by Karpenter (labels), not the DOKS
//     cluster autoscaler.
//  7. Delete the workload.
//  8. Karpenter consolidates and removes the Droplet.
//  9. Cluster returns to the initial size.
//
// ─────────────────────────────────────────────────────────────────────────────
func TestScaleUpAndDown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	defer func() {
		if t.Failed() {
			dumpClusterState(t, context.Background())
		}
	}()

	// ── Step 1: record initial state ─────────────────────────────────────
	initialNodes := getReadyNodeCount(t, ctx)
	t.Logf("Initial ready node count: %d", initialNodes)

	// ── Step 2: create Karpenter resources ───────────────────────────────
	const ncName = "e2e-scale"
	const npName = "e2e-scale"
	createDONodeClass(t, ctx, ncName)
	defer cleanupDONodeClass(t, ncName)

	createNodePool(t, ctx, npName, ncName)
	defer cleanupNodePool(t, npName)

	// ── Step 3: deploy a workload that only runs on Karpenter nodes ──────
	const depName = "e2e-inflate"
	createInflateDeployment(t, ctx, defaultNamespace, depName, npName, 1)
	defer cleanupDeployment(t, defaultNamespace, depName)

	// Wait a moment for the pod to enter Pending (no Karpenter nodes exist yet).
	t.Log("Waiting for pod to become Pending…")
	waitForPendingPods(t, ctx, defaultNamespace, depName, 2*time.Minute)

	// ── Step 4: Karpenter should create a NodeClaim ──────────────────────
	t.Log("Waiting for Karpenter to create a NodeClaim…")
	nodeClaimName := waitForNodeClaim(t, ctx, npName, nodeClaimTimeout)
	t.Logf("NodeClaim created: %s", nodeClaimName)

	// ── Step 5: new node becomes Ready ───────────────────────────────────
	t.Log("Waiting for Karpenter-provisioned node to become Ready…")
	newNodeName := waitForKarpenterNode(t, ctx, nodeReadyTimeout)
	t.Logf("New Karpenter node is Ready: %s", newNodeName)

	// ── Step 6: verify the pod is running ────────────────────────────────
	t.Log("Waiting for inflate deployment to be ready…")
	waitForDeploymentReady(t, ctx, defaultNamespace, depName, deployReadyTimeout)
	t.Log("Deployment is ready — pod is Running on the new node")

	// ── Step 7: verify it was Karpenter, NOT cluster autoscaler ──────────
	verifyKarpenterLabels(t, ctx, newNodeName)
	verifyNodeNotInDOKSPool(t, ctx, newNodeName)
	t.Log("Confirmed: scaling was performed by Karpenter")

	// ── Step 8: remove the workload ──────────────────────────────────────
	t.Log("Deleting inflate deployment to trigger scale-down…")
	deleteDeployment(t, ctx, defaultNamespace, depName)
	waitForDeploymentGone(t, ctx, defaultNamespace, depName, 2*time.Minute)

	// ── Step 9: Karpenter should consolidate and remove the node ─────────
	t.Log("Waiting for Karpenter to remove the node…")
	waitForKarpenterNodeGone(t, ctx, scaleDownTimeout)
	t.Log("Karpenter node removed")

	// ── Step 10: NodeClaim should be deleted ─────────────────────────────
	waitForNodeClaimDeletion(t, ctx, nodeClaimName, scaleDownTimeout)
	t.Log("NodeClaim deleted")

	// ── Step 11: cluster back to original size ───────────────────────────
	finalNodes := getReadyNodeCount(t, ctx)
	t.Logf("Final ready node count: %d (initial was %d)", finalNodes, initialNodes)
	if finalNodes != initialNodes {
		t.Errorf("Expected cluster to return to %d nodes, got %d", initialNodes, finalNodes)
	}

	t.Log("TestScaleUpAndDown PASSED — full lifecycle verified")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestMultiReplicaScaleUp deploys three pods that each require their own node
// and verifies Karpenter provisions multiple nodes concurrently.
// ─────────────────────────────────────────────────────────────────────────────
func TestMultiReplicaScaleUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	defer func() {
		if t.Failed() {
			dumpClusterState(t, context.Background())
		}
	}()

	initialNodes := getReadyNodeCount(t, ctx)

	const ncName = "e2e-multi"
	const npName = "e2e-multi"
	createDONodeClass(t, ctx, ncName)
	defer cleanupDONodeClass(t, ncName)

	createNodePool(t, ctx, npName, ncName)
	defer cleanupNodePool(t, npName)

	// Deploy 3 replicas — each needs its own Karpenter node.
	const depName = "e2e-multi-inflate"
	createInflateDeployment(t, ctx, defaultNamespace, depName, npName, 3)
	defer cleanupDeployment(t, defaultNamespace, depName)

	t.Log("Waiting for pending pods…")
	waitForPendingPods(t, ctx, defaultNamespace, depName, 2*time.Minute)

	// Wait for at least 3 NodeClaims.
	t.Log("Waiting for 3 NodeClaims…")
	err := waitForNodeClaimCount(t, ctx, npName, 3, nodeClaimTimeout)
	if err != nil {
		t.Fatalf("Did not get 3 NodeClaims: %v", err)
	}

	// Wait for 3 Karpenter nodes to be Ready.
	t.Log("Waiting for 3 Karpenter nodes to be Ready…")
	waitForKarpenterNodeCount(t, ctx, 3, nodeReadyTimeout)
	t.Logf("Total nodes: %d (started at %d)", getNodeCount(t, ctx), initialNodes)

	// All replicas should be running.
	t.Log("Waiting for all 3 replicas to be available…")
	waitForDeploymentReady(t, ctx, defaultNamespace, depName, deployReadyTimeout)
	t.Log("All 3 replicas running")

	// Clean up and verify scale-down.
	t.Log("Deleting deployment…")
	deleteDeployment(t, ctx, defaultNamespace, depName)
	waitForDeploymentGone(t, ctx, defaultNamespace, depName, 2*time.Minute)

	t.Log("Waiting for Karpenter nodes to be removed…")
	waitForKarpenterNodeGone(t, ctx, scaleDownTimeout)

	finalNodes := getReadyNodeCount(t, ctx)
	if finalNodes != initialNodes {
		t.Errorf("Expected %d nodes after cleanup, got %d", initialNodes, finalNodes)
	}
	t.Log("TestMultiReplicaScaleUp PASSED")
}

// ─────────────────────────────────────────────────────────────────────────────
// Extra wait helpers used only in this file
// ─────────────────────────────────────────────────────────────────────────────

func waitForNodeClaimCount(t *testing.T, ctx context.Context, nodePoolName string, want int, timeout time.Duration) error {
	t.Helper()
	return waitPoll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		claims, err := dynamicClient.Resource(nodeClaimGVR).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		got := 0
		for _, c := range claims.Items {
			if l := c.GetLabels(); l != nil && l["karpenter.sh/nodepool"] == nodePoolName {
				got++
			}
		}
		t.Logf("  NodeClaims for %q: %d/%d", nodePoolName, got, want)
		return got >= want, nil
	})
}

func waitForKarpenterNodeCount(t *testing.T, ctx context.Context, want int, timeout time.Duration) {
	t.Helper()
	err := waitPoll(ctx, 10*time.Second, timeout, func(ctx context.Context) (bool, error) {
		nodes := getKarpenterNodes(t, ctx)
		ready := 0
		for _, n := range nodes {
			for _, c := range n.Status.Conditions {
				if c.Type == "Ready" && c.Status == "True" {
					ready++
				}
			}
		}
		t.Logf("  Ready Karpenter nodes: %d/%d", ready, want)
		return ready >= want, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for %d Karpenter nodes: %v", want, err)
	}
}

// waitPoll is a thin wrapper around wait.PollUntilContextTimeout.
func waitPoll(ctx context.Context, interval, timeout time.Duration, fn func(context.Context) (bool, error)) error {
	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, fn)
}
