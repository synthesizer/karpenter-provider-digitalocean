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

package fake

import (
	"github.com/digitalocean/godo"
)

// DefaultSizes returns a set of representative DigitalOcean droplet sizes for testing.
func DefaultSizes() []godo.Size {
	return []godo.Size{
		{
			Slug:         "s-1vcpu-1gb",
			Memory:       1024,
			Vcpus:        1,
			Disk:         25,
			PriceMonthly: 6,
			PriceHourly:  0.00893,
			Regions:      []string{"nyc1", "nyc3", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     1,
			Description:  "Basic",
		},
		{
			Slug:         "s-1vcpu-2gb",
			Memory:       2048,
			Vcpus:        1,
			Disk:         50,
			PriceMonthly: 12,
			PriceHourly:  0.01786,
			Regions:      []string{"nyc1", "nyc3", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     2,
			Description:  "Basic",
		},
		{
			Slug:         "s-2vcpu-4gb",
			Memory:       4096,
			Vcpus:        2,
			Disk:         80,
			PriceMonthly: 24,
			PriceHourly:  0.03571,
			Regions:      []string{"nyc1", "nyc3", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     4,
			Description:  "Basic",
		},
		{
			Slug:         "s-4vcpu-8gb",
			Memory:       8192,
			Vcpus:        4,
			Disk:         160,
			PriceMonthly: 48,
			PriceHourly:  0.07143,
			Regions:      []string{"nyc1", "nyc3", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     5,
			Description:  "Basic",
		},
		{
			Slug:         "s-8vcpu-16gb",
			Memory:       16384,
			Vcpus:        8,
			Disk:         320,
			PriceMonthly: 96,
			PriceHourly:  0.14286,
			Regions:      []string{"nyc1", "nyc3", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     6,
			Description:  "Basic",
		},
		{
			Slug:         "g-2vcpu-8gb",
			Memory:       8192,
			Vcpus:        2,
			Disk:         25,
			PriceMonthly: 63,
			PriceHourly:  0.09375,
			Regions:      []string{"nyc1", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     4,
			Description:  "General Purpose",
		},
		{
			Slug:         "g-4vcpu-16gb",
			Memory:       16384,
			Vcpus:        4,
			Disk:         50,
			PriceMonthly: 126,
			PriceHourly:  0.1875,
			Regions:      []string{"nyc1", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     5,
			Description:  "General Purpose",
		},
		{
			Slug:         "c-2vcpu-4gb",
			Memory:       4096,
			Vcpus:        2,
			Disk:         25,
			PriceMonthly: 42,
			PriceHourly:  0.0625,
			Regions:      []string{"nyc1", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     4,
			Description:  "CPU-Optimized",
		},
		{
			Slug:         "m-2vcpu-16gb",
			Memory:       16384,
			Vcpus:        2,
			Disk:         50,
			PriceMonthly: 84,
			PriceHourly:  0.125,
			Regions:      []string{"nyc1", "sfo3", "ams3", "sgp1", "lon1", "fra1"},
			Available:    true,
			Transfer:     4,
			Description:  "Memory-Optimized",
		},
		{
			Slug:         "so-2vcpu-16gb",
			Memory:       16384,
			Vcpus:        2,
			Disk:         300,
			PriceMonthly: 131,
			PriceHourly:  0.19494,
			Regions:      []string{"nyc1", "sfo3", "ams3"},
			Available:    true,
			Transfer:     4,
			Description:  "Storage-Optimized",
		},
	}
}

// DefaultRegions returns a set of DigitalOcean regions for testing.
func DefaultRegions() []godo.Region {
	return []godo.Region{
		{Slug: "nyc1", Name: "New York 1", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "nyc3", Name: "New York 3", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "sfo3", Name: "San Francisco 3", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "ams3", Name: "Amsterdam 3", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "sgp1", Name: "Singapore 1", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "lon1", Name: "London 1", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "fra1", Name: "Frankfurt 1", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "blr1", Name: "Bangalore 1", Available: true, Features: []string{"metadata", "private_networking"}},
		{Slug: "syd1", Name: "Sydney 1", Available: true, Features: []string{"metadata", "private_networking"}},
	}
}
