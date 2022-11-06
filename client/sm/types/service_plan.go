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

const ServicePlansURL = "/v1/service_plans"

// ServicePlan defines the data of a service plan.
type ServicePlan struct {
	ID          string `json:"id,omitempty" yaml:"id,omitempty"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`

	CatalogID     string `json:"catalog_id,omitempty" yaml:"catalog_id,omitempty"`
	CatalogName   string `json:"catalog_name,omitempty" yaml:"catalog_name,omitempty"`
	Free          bool   `json:"free,omitempty" yaml:"free,omitempty"`
	Bindable      bool   `json:"bindable,omitempty" yaml:"bindable,omitempty"`
	PlanUpdatable bool   `json:"plan_updateable,omitempty" yaml:"plan_updateable,omitempty"`

	Metadata json.RawMessage `json:"metadata,omitempty" yaml:"-"`
	Schemas  json.RawMessage `json:"schemas,omitempty" yaml:"-"`

	ServiceOfferingID string `json:"service_offering_id,omitempty" yaml:"service_offering_id,omitempty"`
	Labels            Labels `json:"labels,omitempty" yaml:"labels,omitempty"`
	Ready             bool   `json:"ready,omitempty" yaml:"ready,omitempty"`
}

// ServicePlans wraps an array of service plans
type ServicePlans struct {
	ServicePlans []ServicePlan `json:"items" yaml:"items"`
}
