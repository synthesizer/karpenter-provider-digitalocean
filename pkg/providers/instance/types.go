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

import (
	"time"
)

// Instance represents a DigitalOcean Droplet managed by Karpenter.
type Instance struct {
	// ID is the DigitalOcean Droplet ID.
	ID int

	// Name is the Droplet name (typically the NodeClaim name).
	Name string

	// Region is the region slug (e.g., "nyc1").
	Region string

	// Size is the size slug (e.g., "s-2vcpu-4gb").
	Size string

	// Status is the Droplet status (e.g., "new", "active", "off", "archive").
	Status string

	// PrivateIPv4 is the private IP address within the VPC.
	PrivateIPv4 string

	// PublicIPv4 is the public IPv4 address, if assigned.
	PublicIPv4 string

	// Tags are the Droplet's tags.
	Tags []string

	// CreatedAt is when the Droplet was created.
	CreatedAt time.Time

	// ImageID is the image used to create the Droplet.
	ImageID int

	// VPCUUID is the VPC the Droplet is placed in.
	VPCUUID string
}
