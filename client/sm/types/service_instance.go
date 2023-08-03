/*
 * Copyright 2018 The Service Manager Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package types

import (
	"encoding/json"
)

const ServiceInstancesURL = "/v1/service_instances"

// ServiceInstance defines the data of a service instance.
type ServiceInstance struct {
	ID        string `json:"id,omitempty" yaml:"id,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	CreatedAt string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`

	ServiceID     string `json:"service_id,omitempty" yaml:"service_id,omitempty"`
	ServicePlanID string `json:"service_plan_id,omitempty" yaml:"service_plan_id,omitempty"`
	PlatformID    string `json:"platform_id,omitempty" yaml:"platform_id,omitempty"`

	Parameters json.RawMessage `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Labels     Labels          `json:"labels,omitempty" yaml:"labels,omitempty"`

	MaintenanceInfo json.RawMessage `json:"maintenance_info,omitempty" yaml:"-"`
	Context         json.RawMessage `json:"context,omitempty" yaml:"context,omitempty"`
	PreviousValues  json.RawMessage `json:"-" yaml:"-"`

	Ready  bool `json:"ready" yaml:"ready"`
	Usable bool `json:"usable" yaml:"usable"`
	Shared bool `json:"shared,omitempty" yaml:"shared,omitempty"`

	LastOperation *Operation `json:"last_operation,omitempty" yaml:"last_operation,omitempty"`
}

// ServiceInstances wraps an array of service instances
type ServiceInstances struct {
	ServiceInstances []ServiceInstance `json:"items" yaml:"items"`
}
