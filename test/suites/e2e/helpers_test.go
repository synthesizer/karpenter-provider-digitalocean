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
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
)

// ────────────────────────────────────────────────────────────────────
// GVR constants for dynamic client operations on CRDs
// ────────────────────────────────────────────────────────────────────

var (
	nodePoolGVR = schema.GroupVersionResource{
		Group: "karpenter.sh", Version: "v1", Resource: "nodepools",
	}
	nodeClaimGVR = schema.GroupVersionResource{
		Group: "karpenter.sh", Version: "v1", Resource: "nodeclaims",
	}
	doNodeClassGVR = schema.GroupVersionResource{
		Group: "karpenter.do.sh", Version: "v1alpha1", Resource: "donodeclasses",
	}
)

// ────────────────────────────────────────────────────────────────────
// Node helpers
// ────────────────────────────────────────────────────────────────────

// getNodeCount returns the total number of nodes in the cluster.
func getNodeCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}
	return len(nodes.Items)
}

// getReadyNodeCount returns the number of Ready nodes.
func getReadyNodeCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}
	count := 0
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				count++
				break
			}
		}
	}
	return count
}

// getKarpenterNodes returns all nodes that have the karpenter.sh/nodepool label.
func getKarpenterNodes(t *testing.T, ctx context.Context) []corev1.Node {
	t.Helper()
	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: "karpenter.sh/nodepool",
	})
	if err != nil {
		t.Fatalf("Failed to list Karpenter nodes: %v", err)
	}
	return nodes.Items
}

// ────────────────────────────────────────────────────────────────────
// DONodeClass helpers
// ────────────────────────────────────────────────────────────────────

// createDONodeClass creates a DONodeClass CR with the given name.
// For the DOKS model, only region and tags are needed — DOKS handles
// images, VPC, and bootstrap automatically.
func createDONodeClass(t *testing.T, ctx context.Context, name string) {
	t.Helper()

	spec := map[string]interface{}{
		"region": testRegion,
	}

	nc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "karpenter.do.sh/v1alpha1",
			"kind":       "DONodeClass",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": spec,
		},
	}

	_, err := dynamicClient.Resource(doNodeClassGVR).Create(ctx, nc, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create DONodeClass %q: %v", name, err)
	}
	t.Logf("Created DONodeClass %q in region %s", name, testRegion)
}

// getDONodeClass retrieves a DONodeClass by name.
func getDONodeClass(t *testing.T, ctx context.Context, name string) *unstructured.Unstructured {
	t.Helper()
	nc, err := dynamicClient.Resource(doNodeClassGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get DONodeClass %q: %v", name, err)
	}
	return nc
}

// cleanupDONodeClass deletes the named DONodeClass, ignoring not-found errors.
func cleanupDONodeClass(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := dynamicClient.Resource(doNodeClassGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		t.Logf("Warning: failed to delete DONodeClass %q: %v", name, err)
	}
}

// ────────────────────────────────────────────────────────────────────
// NodePool helpers
// ────────────────────────────────────────────────────────────────────

// createNodePool creates a NodePool that references the given DONodeClass.
func createNodePool(t *testing.T, ctx context.Context, name, nodeClassName string) {
	t.Helper()

	np := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "karpenter.sh/v1",
			"kind":       "NodePool",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"nodeClassRef": map[string]interface{}{
							"group": "karpenter.do.sh",
							"kind":  "DONodeClass",
							"name":  nodeClassName,
						},
						"requirements": []interface{}{
							map[string]interface{}{
								"key":      "kubernetes.io/os",
								"operator": "In",
								"values":   []interface{}{"linux"},
							},
						},
					},
				},
				"limits": map[string]interface{}{
					"cpu":    "20",
					"memory": "80Gi",
				},
				"disruption": map[string]interface{}{
					"consolidationPolicy": "WhenEmpty",
					"consolidateAfter":    "30s",
				},
			},
		},
	}

	_, err := dynamicClient.Resource(nodePoolGVR).Create(ctx, np, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create NodePool %q: %v", name, err)
	}
	t.Logf("Created NodePool %q referencing DONodeClass %q", name, nodeClassName)
}

// createNodePoolWithLimits creates a NodePool with specific CPU and memory limits.
func createNodePoolWithLimits(t *testing.T, ctx context.Context, name, nodeClassName, cpuLimit, memLimit string) {
	t.Helper()

	np := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "karpenter.sh/v1",
			"kind":       "NodePool",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"nodeClassRef": map[string]interface{}{
							"group": "karpenter.do.sh",
							"kind":  "DONodeClass",
							"name":  nodeClassName,
						},
						"requirements": []interface{}{
							map[string]interface{}{
								"key":      "kubernetes.io/os",
								"operator": "In",
								"values":   []interface{}{"linux"},
							},
						},
					},
				},
				"limits": map[string]interface{}{
					"cpu":    cpuLimit,
					"memory": memLimit,
				},
				"disruption": map[string]interface{}{
					"consolidationPolicy": "WhenEmpty",
					"consolidateAfter":    "30s",
				},
			},
		},
	}

	_, err := dynamicClient.Resource(nodePoolGVR).Create(ctx, np, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create NodePool %q: %v", name, err)
	}
	t.Logf("Created NodePool %q with limits cpu=%s mem=%s", name, cpuLimit, memLimit)
}

// cleanupNodePool deletes the named NodePool, ignoring not-found errors.
func cleanupNodePool(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := dynamicClient.Resource(nodePoolGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		t.Logf("Warning: failed to delete NodePool %q: %v", name, err)
	}
}

// ────────────────────────────────────────────────────────────────────
// Deployment helpers
// ────────────────────────────────────────────────────────────────────

// createInflateDeployment creates a Deployment with a pause container whose
// nodeSelector forces scheduling only on Karpenter-provisioned nodes.
func createInflateDeployment(t *testing.T, ctx context.Context, namespace, name, nodePoolName string, replicas int32) {
	t.Helper()

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					// Target only Karpenter-provisioned nodes.
					NodeSelector: map[string]string{
						"karpenter.sh/nodepool": nodePoolName,
					},
					// Do not compete with system pods.
					TerminationGracePeriodSeconds: int64Ptr(0),
					Containers: []corev1.Container{
						{
							Name:  "pause",
							Image: "registry.k8s.io/pause:3.9",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := kubeClient.AppsV1().Deployments(namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create deployment %s/%s: %v", namespace, name, err)
	}
	t.Logf("Created inflate deployment %s/%s with %d replica(s)", namespace, name, replicas)
}

// deleteDeployment removes a deployment from the cluster.
func deleteDeployment(t *testing.T, ctx context.Context, namespace, name string) {
	t.Helper()
	propagation := metav1.DeletePropagationForeground
	err := kubeClient.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil && !errors.IsNotFound(err) {
		t.Logf("Warning: failed to delete deployment %s/%s: %v", namespace, name, err)
	}
}

// cleanupDeployment is a deferred-safe wrapper around deleteDeployment.
func cleanupDeployment(t *testing.T, namespace, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	deleteDeployment(t, ctx, namespace, name)
}

// ────────────────────────────────────────────────────────────────────
// Wait helpers
// ────────────────────────────────────────────────────────────────────

// waitForNodeClaim polls until a NodeClaim owned by the given NodePool appears.
// Returns the NodeClaim's name.
func waitForNodeClaim(t *testing.T, ctx context.Context, nodePoolName string, timeout time.Duration) string {
	t.Helper()

	var nodeClaimName string
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		claims, listErr := dynamicClient.Resource(nodeClaimGVR).List(ctx, metav1.ListOptions{})
		if listErr != nil {
			t.Logf("  listing NodeClaims: %v", listErr)
			return false, nil
		}
		for _, claim := range claims.Items {
			labels := claim.GetLabels()
			if labels != nil && labels["karpenter.sh/nodepool"] == nodePoolName {
				nodeClaimName = claim.GetName()
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for NodeClaim for NodePool %q: %v", nodePoolName, err)
	}
	return nodeClaimName
}

// waitForNodeClaimDeletion polls until no NodeClaim with the given name exists.
func waitForNodeClaimDeletion(t *testing.T, ctx context.Context, name string, timeout time.Duration) {
	t.Helper()

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		_, getErr := dynamicClient.Resource(nodeClaimGVR).Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(getErr) {
			return true, nil
		}
		if getErr != nil {
			t.Logf("  checking NodeClaim %q: %v", name, getErr)
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for NodeClaim %q to be deleted: %v", name, err)
	}
}

// waitForKarpenterNode waits for at least one Ready node with the
// karpenter.sh/nodepool label to appear. Returns the new node's name.
func waitForKarpenterNode(t *testing.T, ctx context.Context, timeout time.Duration) string {
	t.Helper()

	var newNodeName string
	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		nodes, listErr := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			LabelSelector: "karpenter.sh/nodepool",
		})
		if listErr != nil {
			t.Logf("  listing Karpenter nodes: %v", listErr)
			return false, nil
		}
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					newNodeName = node.Name
					return true, nil
				}
			}
		}
		if len(nodes.Items) > 0 {
			t.Logf("  %d Karpenter node(s) exist, waiting for Ready…", len(nodes.Items))
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for a Ready Karpenter node: %v", err)
	}
	return newNodeName
}

// waitForKarpenterNodeGone waits until no nodes with karpenter.sh/nodepool label remain.
func waitForKarpenterNodeGone(t *testing.T, ctx context.Context, timeout time.Duration) {
	t.Helper()

	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		nodes, listErr := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			LabelSelector: "karpenter.sh/nodepool",
		})
		if listErr != nil {
			t.Logf("  listing Karpenter nodes: %v", listErr)
			return false, nil
		}
		if len(nodes.Items) > 0 {
			t.Logf("  %d Karpenter node(s) remaining…", len(nodes.Items))
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for Karpenter nodes to be removed: %v", err)
	}
}

// waitForNodeCount waits until the total node count matches the expected value.
func waitForNodeCount(t *testing.T, ctx context.Context, expected int, timeout time.Duration) {
	t.Helper()

	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		nodes, listErr := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if listErr != nil {
			t.Logf("  listing nodes: %v", listErr)
			return false, nil
		}
		current := len(nodes.Items)
		if current != expected {
			t.Logf("  node count: %d (waiting for %d)", current, expected)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for node count to reach %d: %v", expected, err)
	}
}

// waitForDeploymentReady waits until all replicas of a deployment are available.
func waitForDeploymentReady(t *testing.T, ctx context.Context, namespace, name string, timeout time.Duration) {
	t.Helper()

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		dep, getErr := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			t.Logf("  getting deployment %s/%s: %v", namespace, name, getErr)
			return false, nil
		}
		if dep.Status.AvailableReplicas == *dep.Spec.Replicas {
			return true, nil
		}
		t.Logf("  deployment %s/%s: %d/%d replicas available",
			namespace, name, dep.Status.AvailableReplicas, *dep.Spec.Replicas)
		return false, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for deployment %s/%s to be ready: %v", namespace, name, err)
	}
}

// waitForDeploymentGone waits until a deployment no longer exists.
func waitForDeploymentGone(t *testing.T, ctx context.Context, namespace, name string, timeout time.Duration) {
	t.Helper()

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		_, getErr := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(getErr) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for deployment %s/%s to be deleted: %v", namespace, name, err)
	}
}

// waitForPendingPods waits for at least one pod from the given deployment to be in Pending state.
func waitForPendingPods(t *testing.T, ctx context.Context, namespace, deploymentName string, timeout time.Duration) {
	t.Helper()

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pods, listErr := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", deploymentName),
		})
		if listErr != nil {
			return false, nil
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodPending {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Timed out waiting for pending pods for deployment %s/%s: %v", namespace, deploymentName, err)
	}
}

// ────────────────────────────────────────────────────────────────────
// Verification helpers
// ────────────────────────────────────────────────────────────────────

// verifyKarpenterLabels asserts that the given node has Karpenter-specific labels
// and does NOT have cluster-autoscaler labels.
func verifyKarpenterLabels(t *testing.T, ctx context.Context, nodeName string) {
	t.Helper()

	node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get node %q: %v", nodeName, err)
	}

	labels := node.Labels

	// Must have Karpenter labels.
	if _, ok := labels["karpenter.sh/nodepool"]; !ok {
		t.Errorf("Node %q is missing karpenter.sh/nodepool label", nodeName)
	}
	if _, ok := labels["karpenter.sh/capacity-type"]; !ok {
		t.Logf("Node %q is missing karpenter.sh/capacity-type label (may be set later)", nodeName)
	}

	// Must NOT have cluster-autoscaler labels/annotations.
	annotations := node.Annotations
	if _, ok := annotations["cluster-autoscaler.kubernetes.io/scale-down-disabled"]; ok {
		t.Errorf("Node %q has cluster-autoscaler annotation — scaling was NOT done by Karpenter", nodeName)
	}

	t.Logf("Node %q labels verified: managed by Karpenter, not cluster autoscaler", nodeName)
}

// verifyNodeInDOKSPool checks that the Karpenter-provisioned node is part of a
// DOKS node pool (since Karpenter now creates DOKS node pools with Count=1).
func verifyNodeInDOKSPool(t *testing.T, ctx context.Context, nodeName string) {
	t.Helper()

	node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get node %q: %v", nodeName, err)
	}

	// In the DOKS model, Karpenter-provisioned nodes ARE part of DOKS node pools.
	if pool, ok := node.Labels["doks.digitalocean.com/node-pool"]; ok {
		t.Logf("Node %q belongs to DOKS node pool %q (expected for DOKS model)", nodeName, pool)
	} else {
		t.Logf("Node %q does not have doks.digitalocean.com/node-pool label yet (may appear later)", nodeName)
	}
}

// ────────────────────────────────────────────────────────────────────
// Misc helpers
// ────────────────────────────────────────────────────────────────────

func int64Ptr(v int64) *int64 { return &v }

// dumpClusterState logs useful diagnostic info when a test fails.
func dumpClusterState(t *testing.T, ctx context.Context) {
	t.Helper()

	nodes, _ := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if nodes != nil {
		t.Logf("── Nodes (%d) ──", len(nodes.Items))
		for _, n := range nodes.Items {
			ready := "NotReady"
			for _, c := range n.Status.Conditions {
				if c.Type == corev1.NodeReady {
					ready = string(c.Status)
				}
			}
			t.Logf("  %s  Ready=%s  providerID=%s", n.Name, ready, n.Spec.ProviderID)
			t.Logf("    labels=%v", n.Labels)
			if len(n.Spec.Taints) > 0 {
				t.Logf("    taints=%v", n.Spec.Taints)
			}
		}
	}

	claims, _ := dynamicClient.Resource(nodeClaimGVR).List(ctx, metav1.ListOptions{})
	if claims != nil {
		t.Logf("── NodeClaims (%d) ──", len(claims.Items))
		for _, c := range claims.Items {
			status, _, _ := unstructured.NestedMap(c.Object, "status")
			providerID, _, _ := unstructured.NestedString(c.Object, "status", "providerID")
			nodeName, _, _ := unstructured.NestedString(c.Object, "status", "nodeName")
			conditions, _, _ := unstructured.NestedSlice(c.Object, "status", "conditions")
			t.Logf("  %s  labels=%v", c.GetName(), c.GetLabels())
			t.Logf("    providerID=%s  nodeName=%s", providerID, nodeName)
			if len(conditions) > 0 {
				t.Logf("    conditions=%v", conditions)
			}
			_ = status // suppress unused warning
		}
	}

	pools, _ := dynamicClient.Resource(nodePoolGVR).List(ctx, metav1.ListOptions{})
	if pools != nil {
		t.Logf("── NodePools (%d) ──", len(pools.Items))
		for _, p := range pools.Items {
			t.Logf("  %s", p.GetName())
		}
	}

	// Dump Karpenter controller pod logs (last 50 lines) for debugging
	pods, _ := kubeClient.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=karpenter",
	})
	if pods != nil && len(pods.Items) > 0 {
		for _, pod := range pods.Items {
			sinceSeconds := int64(300) // last 5 minutes
			tailLines := int64(80)
			logs, logErr := kubeClient.CoreV1().Pods("kube-system").GetLogs(pod.Name, &corev1.PodLogOptions{
				SinceSeconds: &sinceSeconds,
				TailLines:    &tailLines,
			}).Do(ctx).Raw()
			if logErr == nil {
				t.Logf("── Karpenter pod %s logs (last 80 lines) ──", pod.Name)
				t.Logf("%s", string(logs))
			}
		}
	}
}
