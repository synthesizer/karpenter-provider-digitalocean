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

package pricing

import (
	"testing"
)

func TestInstanceTypePrice(t *testing.T) {
	provider := &DefaultProvider{
		prices: map[priceKey]float64{
			{size: "s-1vcpu-1gb", region: "nyc1"}: 0.00893,
			{size: "s-2vcpu-4gb", region: "nyc1"}: 0.03571,
			{size: "s-2vcpu-4gb", region: "sfo3"}: 0.03571,
			{size: "g-2vcpu-8gb", region: "nyc1"}: 0.09375,
		},
	}

	tests := []struct {
		name      string
		size      string
		region    string
		wantPrice float64
		wantOk    bool
	}{
		{
			name:      "existing price",
			size:      "s-1vcpu-1gb",
			region:    "nyc1",
			wantPrice: 0.00893,
			wantOk:    true,
		},
		{
			name:      "same size different region",
			size:      "s-2vcpu-4gb",
			region:    "sfo3",
			wantPrice: 0.03571,
			wantOk:    true,
		},
		{
			name:      "unknown size",
			size:      "s-16vcpu-32gb",
			region:    "nyc1",
			wantPrice: 0,
			wantOk:    false,
		},
		{
			name:      "unknown region",
			size:      "s-1vcpu-1gb",
			region:    "unknown",
			wantPrice: 0,
			wantOk:    false,
		},
		{
			name:      "general purpose size",
			size:      "g-2vcpu-8gb",
			region:    "nyc1",
			wantPrice: 0.09375,
			wantOk:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, ok := provider.InstanceTypePrice(tt.size, tt.region)
			if ok != tt.wantOk {
				t.Errorf("InstanceTypePrice() ok = %v, wantOk %v", ok, tt.wantOk)
			}
			if price != tt.wantPrice {
				t.Errorf("InstanceTypePrice() price = %v, wantPrice %v", price, tt.wantPrice)
			}
		})
	}
}
