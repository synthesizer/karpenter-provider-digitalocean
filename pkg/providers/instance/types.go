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

package instance

import "time"

// Instance represents a DigitalOcean Kubernetes node managed by Karpenter.
// Each Instance corresponds to a single node within a DOKS Node Pool that
// was created by Karpenter (1 node pool = 1 node = 1 NodeClaim).
type Instance struct {
	// NodePoolID is the DOKS Node Pool ID this node belongs to.
	// This is the primary identifier used for deletion (delete the whole pool).
	NodePoolID string

	// DropletID is the underlying DigitalOcean Droplet ID for this node.
	// Used to construct the Kubernetes provider ID (digitalocean://<dropletID>).
	DropletID string

	// Name is the node name as assigned by DOKS.
	Name string

	// Region is the region slug (e.g., "nyc1").
	Region string

	// Size is the Droplet size slug (e.g., "s-2vcpu-4gb").
	Size string

	// Status is the DOKS node status (e.g., "provisioning", "running", "draining", "deleting").
	Status string

	// Labels are the Kubernetes labels from the node pool, propagated to the node.
	Labels map[string]string

	// Tags are the DOKS Node Pool's tags.
	Tags []string

	// CreatedAt is the timestamp when the node was created.
	CreatedAt time.Time
}
