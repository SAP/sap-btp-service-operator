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

const ResourceOperationsURL = "/operations"

// Operation defines the data of a operation.
type Operation struct {
	ID           string            `json:"id,omitempty" yaml:"id,omitempty"`
	Description  string            `json:"description,omitempty" yaml:"description,omitempty"`
	Type         OperationCategory `json:"type,omitempty" yaml:"type,omitempty"`
	State        OperationState    `json:"state,omitempty" yaml:"state,omitempty"`
	ResourceID   string            `json:"resource_id,omitempty" yaml:"resource_id,omitempty"`
	ResourceType string            `json:"resource_type,omitempty" yaml:"resource_type,omitempty"`
	Errors       json.RawMessage   `json:"errors,omitempty" yaml:"errors,omitempty"`
	Created      string            `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	Updated      string            `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	Labels       Labels            `json:"labels,omitempty" yaml:"labels,omitempty"`
}
