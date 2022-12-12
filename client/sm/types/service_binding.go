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

const ServiceBindingsURL = "/v1/service_bindings"

// ServiceBinding defines the data of a service instance.
type ServiceBinding struct {
	ID             string `json:"id,omitempty" yaml:"id,omitempty"`
	Name           string `json:"name,omitempty" yaml:"name,omitempty"`
	CreatedAt      string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	Labels         Labels `json:"labels,omitempty" yaml:"labels,omitempty"`
	PagingSequence int64  `json:"-" yaml:"-"`

	Credentials json.RawMessage `json:"credentials,omitempty" yaml:"credentials,omitempty"`

	ServiceInstanceID   string `json:"service_instance_id" yaml:"service_instance_id,omitempty"`
	ServiceInstanceName string `json:"service_instance_name,omitempty" yaml:"service_instance_name,omitempty"`

	SyslogDrainURL  string          `json:"syslog_drain_url,omitempty" yaml:"syslog_drain_url,omitempty"`
	RouteServiceURL string          `json:"route_service_url,omitempty"`
	VolumeMounts    json.RawMessage `json:"-" yaml:"-"`
	Endpoints       json.RawMessage `json:"-" yaml:"-"`
	Context         json.RawMessage `json:"context,omitempty" yaml:"context,omitempty"`
	Parameters      json.RawMessage `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	BindResource    json.RawMessage `json:"-" yaml:"-"`

	Ready bool `json:"ready,omitempty" yaml:"ready,omitempty"`

	LastOperation *Operation `json:"last_operation,omitempty" yaml:"last_operation,omitempty"`
}

// ServiceBindings wraps an array of service bindings
type ServiceBindings struct {
	ServiceBindings []ServiceBinding `json:"items" yaml:"items"`
	Vertical        bool             `json:"-" yaml:"-"`
}
