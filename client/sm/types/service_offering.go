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

const ServiceOfferingsURL = "/v1/service_offerings"

// ServiceOffering defines the data of a service offering.
type ServiceOffering struct {
	ID          string `json:"id,omitempty" yaml:"id,omitempty"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`

	Bindable             bool   `json:"bindable,omitempty" yaml:"bindable,omitempty"`
	InstancesRetrievable bool   `json:"instances_retrievable,omitempty" yaml:"instances_retrievable,omitempty"`
	BindingsRetrievable  bool   `json:"bindings_retrievable,omitempty" yaml:"bindings_retrievable,omitempty"`
	AllowContextUpdates  bool   `json:"allow_context_updates,omitempty" yaml:"allow_context_updates,omitempty"`
	PlanUpdatable        bool   `json:"plan_updateable,omitempty" yaml:"plan_updateable,omitempty"`
	CatalogID            string `json:"catalog_id,omitempty" yaml:"catalog_id,omitempty"`
	CatalogName          string `json:"catalog_name,omitempty" yaml:"catalog_name,omitempty"`

	Tags     json.RawMessage `json:"tags,omitempty" yaml:"-"`
	Requires json.RawMessage `json:"requires,omitempty" yaml:"-"`
	Metadata json.RawMessage `json:"metadata,omitempty" yaml:"-"`

	BrokerID   string        `json:"broker_id,omitempty" yaml:"broker_id,omitempty"`
	BrokerName string        `json:"broker_name,omitempty" yaml:"broker_name,omitempty"`
	Plans      []ServicePlan `json:"plans,omitempty" yaml:"plans,omitempty"`
	Labels     Labels        `json:"labels,omitempty" yaml:"labels,omitempty"`
	Ready      bool          `json:"ready,omitempty" yaml:"ready,omitempty"`
	DataCenter string        `json:"data_center,omitempty" yaml:"data_center,omitempty"`
}

// ServiceOfferings wraps an array of service offerings
type ServiceOfferings struct {
	ServiceOfferings []ServiceOffering `json:"items" yaml:"items"`
}
