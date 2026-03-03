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

package operator

import (
	"context"
	"fmt"
	"os"

	"github.com/awslabs/operatorpkg/controller"
	"github.com/awslabs/operatorpkg/status"
	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/digitalocean/karpenter-provider-digitalocean/pkg/apis/v1alpha1"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/cloudprovider"
	nodeclaimcontroller "github.com/digitalocean/karpenter-provider-digitalocean/pkg/controllers/nodeclaim"
	nodeclasscontroller "github.com/digitalocean/karpenter-provider-digitalocean/pkg/controllers/nodeclass"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instance"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/instancetype"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/loadbalancer"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/pricing"
	"github.com/digitalocean/karpenter-provider-digitalocean/pkg/providers/region"

	karpenteroperator "sigs.k8s.io/karpenter/pkg/operator"
)

const (
	// EnvDOAccessToken is the environment variable for the DigitalOcean API token.
	EnvDOAccessToken = "DIGITALOCEAN_ACCESS_TOKEN"

	// EnvClusterName is the environment variable for the cluster name.
	EnvClusterName = "CLUSTER_NAME"

	// EnvClusterID is the environment variable for the DOKS cluster UUID.
	EnvClusterID = "CLUSTER_ID"

	// EnvClusterRegion is the environment variable for the DOKS cluster region.
	EnvClusterRegion = "CLUSTER_REGION"
)

// Operator holds all DigitalOcean-specific provider instances and wraps
// the core Karpenter operator. It initializes the DigitalOcean API client,
// creates all DO-specific providers, and constructs the CloudProvider
// implementation that bridges Karpenter core to DigitalOcean.
type Operator struct {
	*karpenteroperator.Operator

	DOClient             *godo.Client
	InstanceProvider     instance.Provider
	InstanceTypeProvider instancetype.Provider
	PricingProvider      pricing.Provider
	RegionProvider       region.Provider
	LoadBalancerProvider loadbalancer.Provider

	CloudProvider *cloudprovider.CloudProvider
	ClusterName   string
	ClusterID     string
	ClusterRegion string
}

// NewOperator creates a new DigitalOcean operator with all providers initialized.
// It wraps the core Karpenter operator (which provides the controller-runtime
// manager, options, health checks, indexers, etc.) and adds DigitalOcean-specific
// providers on top.
func NewOperator(ctx context.Context, coreOperator *karpenteroperator.Operator) (*Operator, error) {
	logger := log.FromContext(ctx)

	// Get DigitalOcean API token
	token := os.Getenv(EnvDOAccessToken)
	if token == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvDOAccessToken)
	}

	// Get cluster name
	clusterName := os.Getenv(EnvClusterName)
	if clusterName == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvClusterName)
	}

	// Get DOKS cluster ID (required for node pool API calls)
	clusterID := os.Getenv(EnvClusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvClusterID)
	}

	// Get DOKS cluster region
	clusterRegion := os.Getenv(EnvClusterRegion)
	if clusterRegion == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvClusterRegion)
	}

	// Create DigitalOcean API client
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthClient := oauth2.NewClient(ctx, tokenSource)
	doClient := godo.NewClient(oauthClient)

	// Initialize providers
	pricingProvider := pricing.NewDefaultProvider(doClient)

	// Seed pricing data
	if err := pricingProvider.LivePricing(ctx); err != nil {
		// Log warning but don't fail — pricing will fall back to API-reported prices
		logger.Error(err, "WARNING: failed to seed pricing data, prices may be unavailable")
	}

	instanceProvider := instance.NewDefaultProvider(doClient, clusterID, clusterName, clusterRegion)
	instanceTypeProvider := instancetype.NewDefaultProvider(doClient, pricingProvider)
	regionProvider := region.NewDefaultProvider(doClient)
	loadBalancerProvider := loadbalancer.NewDefaultProvider(doClient)

	// Create the CloudProvider that bridges Karpenter core to DigitalOcean
	cp := cloudprovider.New(
		coreOperator.GetClient(),
		instanceProvider,
		instanceTypeProvider,
	)

	logger.Info("DigitalOcean operator initialized",
		"clusterName", clusterName,
		"clusterID", clusterID,
		"clusterRegion", clusterRegion,
	)

	return &Operator{
		Operator:             coreOperator,
		DOClient:             doClient,
		InstanceProvider:     instanceProvider,
		InstanceTypeProvider: instanceTypeProvider,
		PricingProvider:      pricingProvider,
		RegionProvider:       regionProvider,
		LoadBalancerProvider: loadBalancerProvider,
		CloudProvider:        cp,
		ClusterName:          clusterName,
		ClusterID:            clusterID,
		ClusterRegion:        clusterRegion,
	}, nil
}

// NewControllers returns the DigitalOcean-specific controllers that should be
// registered alongside the core Karpenter controllers. These handle:
//   - DONodeClass reconciliation (region validation, status updates)
//   - NodeClaim garbage collection (orphaned node pool cleanup)
//   - DONodeClass status condition management
func (o *Operator) NewControllers(kubeClient client.Client) []controller.Controller {
	return []controller.Controller{
		// DONodeClass controller — validates region, manages status
		nodeclasscontroller.NewController(kubeClient, o.RegionProvider, o.ClusterRegion),

		// NodeClaim garbage collection — deletes orphaned DOKS node pools
		nodeclaimcontroller.NewGarbageCollectionController(kubeClient, o.InstanceProvider),

		// DONodeClass status controller — manages status conditions using operatorpkg
		status.NewController[*v1alpha1.DONodeClass](kubeClient, o.GetEventRecorderFor("karpenter-do")),
	}
}
