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

package main

import (
	"os"

	"sigs.k8s.io/controller-runtime/pkg/log"

	// Blank import to trigger init() which registers DONodeClass types
	// into the k8s.io/client-go/kubernetes/scheme.Scheme. The core
	// Karpenter operator uses this scheme when creating the controller-runtime
	// manager, so our types must be registered before NewOperator() is called.
	_ "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/operator"

	karpentercontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	karpenteroperator "sigs.k8s.io/karpenter/pkg/operator"
)

func main() {
	// The core Karpenter operator sets up:
	//   - Context with parsed options (flags, env vars)
	//   - Logging (via zapr)
	//   - Controller-runtime manager with leader election, metrics, health checks
	//   - Field indexers for Pods, Nodes, NodeClaims, NodePools
	//   - Ready/health check endpoints
	ctx, coreOp := karpenteroperator.NewOperator()
	logger := log.FromContext(ctx)

	// Create the DigitalOcean-specific operator. This:
	//   - Reads DO_ACCESS_TOKEN and CLUSTER_NAME from environment
	//   - Initializes the DigitalOcean API client
	//   - Creates all DO-specific providers (instance, instance type, image, pricing, region, VPC, LB)
	//   - Constructs the CloudProvider that bridges Karpenter core to DigitalOcean
	doOp, err := operator.NewOperator(ctx, coreOp)
	if err != nil {
		logger.Error(err, "failed to create DigitalOcean operator")
		os.Exit(1)
	}

	// Register all controllers and start the manager.
	//
	// Core Karpenter controllers handle:
	//   - Provisioning (scheduling pods → creating NodeClaims)
	//   - Disruption (consolidation, drift, expiration)
	//   - Node lifecycle (termination, hydration, health)
	//   - NodeClaim lifecycle (launch, registration, initialization)
	//   - NodePool management (hashing, validation, readiness, counters)
	//   - Metrics (node, pod, nodepool)
	//   - Status conditions (NodeClaim, NodePool, Node)
	//   - State management (cluster state informers)
	//
	// DigitalOcean-specific controllers handle:
	//   - DONodeClass reconciliation (image resolution, VPC validation)
	//   - NodeClaim garbage collection (orphaned droplet cleanup)
	//   - DONodeClass status conditions (via operatorpkg status controller)
	// Note: We pass coreOp.Manager (not coreOp directly) because the core
	// Operator shadows manager.Manager.Start with its own Start method
	// that has a different signature.
	coreOp.
		WithControllers(ctx, karpentercontrollers.NewControllers(
			ctx,
			coreOp.Manager,
			coreOp.Clock,
			coreOp.GetClient(),
			coreOp.EventRecorder,
			doOp.CloudProvider,
		)...).
		WithControllers(ctx, doOp.NewControllers(coreOp.GetClient())...).
		Start(ctx)
}
