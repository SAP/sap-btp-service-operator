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

package sm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Peripli/service-manager/pkg/log"
	"github.com/Peripli/service-manager/pkg/util"
	"github.com/Peripli/service-manager/pkg/web"
	"github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/SAP/sap-btp-service-operator/internal/auth"
	"github.com/SAP/sap-btp-service-operator/internal/httputil"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const tokenURLSuffix = "/oauth/token"

// Client should be implemented by SM clients
//go:generate counterfeiter . Client
type Client interface {
	ListInstances(*Parameters) (*types.ServiceInstances, error)
	GetInstanceByID(string, *Parameters) (*types.ServiceInstance, error)
	UpdateInstance(id string, updatedInstance *types.ServiceInstance, serviceName string, planName string, q *Parameters) (*types.ServiceInstance, string, error)
	Provision(*types.ServiceInstance, string, string, *Parameters) (string, string, error)
	Deprovision(string, *Parameters) (string, error)

	ListBindings(*Parameters) (*types.ServiceBindings, error)
	GetBindingByID(string, *Parameters) (*types.ServiceBinding, error)
	Bind(*types.ServiceBinding, *Parameters) (*types.ServiceBinding, string, error)
	Unbind(string, *Parameters) (string, error)

	ListOfferings(*Parameters) (*types.ServiceOfferings, error)
	ListPlans(*Parameters) (*types.ServicePlans, error)

	Status(string, *Parameters) (*types.Operation, error)

	// Call makes HTTP request to the Service Manager server with authentication.
	// It should be used only in case there is no already implemented method for such an operation
	Call(method string, smpath string, body io.Reader, q *Parameters) (*http.Response, error)
}
type ServiceManagerError struct {
	Message    string
	StatusCode int
}

func (e *ServiceManagerError) Error() string {
	return e.Message
}

type serviceManagerClient struct {
	Context    context.Context
	Config     *ClientConfig
	HTTPClient auth.HTTPClient
}

// NewClientWithAuth returns new SM Client configured with the provided configuration
func NewClient(ctx context.Context, config *ClientConfig, httpClient auth.HTTPClient) Client {
	if httpClient != nil {
		return &serviceManagerClient{Context: ctx, Config: config, HTTPClient: httpClient}
	}
	ccConfig := &clientcredentials.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		TokenURL:     config.TokenURL + tokenURLSuffix,
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	authClient := auth.NewAuthClient(ccConfig, config.SSLDisabled)
	return &serviceManagerClient{Context: ctx, Config: config, HTTPClient: authClient}
}

// Provision provisions a new service instance in service manager
func (client *serviceManagerClient) Provision(instance *types.ServiceInstance, serviceName string, planName string, q *Parameters) (string, string, error) {
	var newInstance *types.ServiceInstance
	var instanceID string
	if len(serviceName) == 0 || len(planName) == 0 {
		return "", "", fmt.Errorf("missing field values. Specify service name and plan name for the instance '%s'", instance.Name)
	}

	planID, err := client.getAndValidatePlanID(instance.ServicePlanID, serviceName, planName)
	if err != nil {
		return "", "", err
	}

	instance.ServicePlanID = planID

	location, err := client.register(instance, web.ServiceInstancesURL, q, &newInstance)
	if err != nil {
		return "", "", err
	}
	if len(location) > 0 {
		instanceID = ExtractInstanceID(location)
		if len(instanceID) == 0 {
			return "", "", fmt.Errorf("failed to extract the service instance ID from the async operation URL: %s", location)
		}
	} else if newInstance != nil {
		instanceID = newInstance.ID
	}
	return instanceID, location, nil
}

// Bind creates binding to an instance in service manager
func (client *serviceManagerClient) Bind(binding *types.ServiceBinding, q *Parameters) (*types.ServiceBinding, string, error) {
	var newBinding *types.ServiceBinding
	location, err := client.register(binding, web.ServiceBindingsURL, q, &newBinding)
	if err != nil {
		return nil, "", err
	}
	return newBinding, location, nil
}

// ListInstances returns service instances registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) ListInstances(q *Parameters) (*types.ServiceInstances, error) {
	instances := &types.ServiceInstances{}
	err := client.list(&instances.ServiceInstances, web.ServiceInstancesURL, q)

	return instances, err
}

// GetInstanceByID returns instance registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) GetInstanceByID(id string, q *Parameters) (*types.ServiceInstance, error) {
	instance := &types.ServiceInstance{}
	err := client.get(instance, web.ServiceInstancesURL+"/"+id, q)

	return instance, err
}

// ListBindings returns service bindings registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) ListBindings(q *Parameters) (*types.ServiceBindings, error) {
	bindings := &types.ServiceBindings{}
	err := client.list(&bindings.ServiceBindings, web.ServiceBindingsURL, q)

	return bindings, err
}

// GetBindingByID returns binding registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) GetBindingByID(id string, q *Parameters) (*types.ServiceBinding, error) {
	binding := &types.ServiceBinding{}
	err := client.get(binding, web.ServiceBindingsURL+"/"+id, q)

	return binding, err
}

func (client *serviceManagerClient) Status(url string, q *Parameters) (*types.Operation, error) {
	operation := &types.Operation{}
	err := client.get(operation, url, q)

	return operation, err
}

func (client *serviceManagerClient) Deprovision(id string, q *Parameters) (string, error) {
	return client.delete(web.ServiceInstancesURL+"/"+id, q)
}

func (client *serviceManagerClient) Unbind(id string, q *Parameters) (string, error) {
	return client.delete(web.ServiceBindingsURL+"/"+id, q)
}

func (client *serviceManagerClient) UpdateInstance(id string, updatedInstance *types.ServiceInstance, serviceName string, planName string, q *Parameters) (*types.ServiceInstance, string, error) {
	var result *types.ServiceInstance

	planID, err := client.getAndValidatePlanID(updatedInstance.ServicePlanID, serviceName, planName)
	if err != nil {
		return nil, "", err
	}
	updatedInstance.ServicePlanID = planID
	location, err := client.update(updatedInstance, web.ServiceInstancesURL, id, q, &result)
	if err != nil {
		return nil, "", err
	}
	return result, location, nil
}

func (client *serviceManagerClient) list(result interface{}, url string, q *Parameters) error {
	fullURL := httputil.NormalizeURL(client.Config.URL) + BuildURL(url, q)
	return util.ListAll(client.Context, client.HTTPClient.Do, fullURL, result)
}

func (client *serviceManagerClient) ListOfferings(q *Parameters) (*types.ServiceOfferings, error) {
	offerings := &types.ServiceOfferings{}
	err := client.list(&offerings.ServiceOfferings, web.ServiceOfferingsURL, q)

	return offerings, err
}

func (client *serviceManagerClient) ListPlans(q *Parameters) (*types.ServicePlans, error) {
	plans := &types.ServicePlans{}
	err := client.list(&plans.ServicePlans, web.ServicePlansURL, q)

	return plans, err
}

func (client *serviceManagerClient) register(resource interface{}, url string, q *Parameters, result interface{}) (string, error) {
	requestBody, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}

	buffer := bytes.NewBuffer(requestBody)
	response, err := client.Call(http.MethodPost, url, buffer, q)
	if err != nil {
		return "", err
	}

	switch response.StatusCode {
	case http.StatusCreated:
		return "", httputil.UnmarshalResponse(response, &result)
	case http.StatusAccepted:
		return response.Header.Get("Location"), nil
	default:
		return "", handleFailedResponse(response)
	}
}

func (client *serviceManagerClient) delete(url string, q *Parameters) (string, error) {
	response, err := client.Call(http.MethodDelete, url, nil, q)
	if err != nil {
		return "", err
	}
	switch response.StatusCode {
	case http.StatusGone:
		fallthrough
	case http.StatusNotFound:
		fallthrough
	case http.StatusOK:
		return "", nil
	case http.StatusAccepted:
		return response.Header.Get("Location"), nil
	default:
		return "", handleFailedResponse(response)
	}
}

func (client *serviceManagerClient) get(result interface{}, url string, q *Parameters) error {
	response, err := client.Call(http.MethodGet, url, nil, q)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return handleFailedResponse(response)
	}
	return httputil.UnmarshalResponse(response, &result)
}

func (client *serviceManagerClient) update(resource interface{}, url string, id string, q *Parameters, result interface{}) (string, error) {
	requestBody, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}
	buffer := bytes.NewBuffer(requestBody)
	response, err := client.Call(http.MethodPatch, url+"/"+id, buffer, q)
	if err != nil {
		return "", err
	}

	switch response.StatusCode {
	case http.StatusOK:
		return "", httputil.UnmarshalResponse(response, &result)
	case http.StatusAccepted:
		return response.Header.Get("Location"), nil
	default:
		return "", handleFailedResponse(response)
	}
}

func handleFailedResponse(response *http.Response) error {
	err := util.HandleResponseError(response)
	return &ServiceManagerError{
		StatusCode: response.StatusCode,
		Message:    err.Error(),
	}
}

func (client *serviceManagerClient) Call(method string, smpath string, body io.Reader, q *Parameters) (*http.Response, error) {
	fullURL := httputil.NormalizeURL(client.Config.URL) + BuildURL(smpath, q)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	log.C(client.Context).Debugf("Sending request %s %s", req.Method, req.URL)
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (client *serviceManagerClient) getAndValidatePlanID(planID string, serviceName string, planName string) (string, error) {
	query := &Parameters{
		FieldQuery: []string{fmt.Sprintf("catalog_name eq '%s'", serviceName)},
	}
	offerings, err := client.ListOfferings(query)
	if err != nil {
		return "", err
	}

	var commaSepOfferingIds string
	if len(offerings.ServiceOfferings) == 0 {
		return "", fmt.Errorf("couldn't find the service offering '%s'", serviceName)
	}

	serviceOfferingIds := make([]string, 0, len(offerings.ServiceOfferings))
	for _, svc := range offerings.ServiceOfferings {
		serviceOfferingIds = append(serviceOfferingIds, svc.ID)
	}
	commaSepOfferingIds = "'" + strings.Join(serviceOfferingIds, "', '") + "'"

	query = &Parameters{
		FieldQuery: []string{fmt.Sprintf("catalog_name eq '%s'", planName), fmt.Sprintf("service_offering_id in (%s)", commaSepOfferingIds)},
	}

	plans, err := client.ListPlans(query)
	if err != nil {
		return "", err
	}
	if len(plans.ServicePlans) == 0 {
		return "", fmt.Errorf("couldn't find the service plan '%s' for the service offering '%s'", planName, serviceName)
	} else if len(plans.ServicePlans) == 1 && len(planID) == 0 {
		return plans.ServicePlans[0].ID, nil
	} else {
		for _, plan := range plans.ServicePlans {
			if plan.ID == planID {
				return plan.ID, nil
			}
		}
	}

	if len(planID) > 0 {
		err = fmt.Errorf("the provided plan ID '%s' doesn't match the provided offering name '%s' and plan name '%s", planID, serviceName, planName)
	} else {
		err = fmt.Errorf("ambiguity error: found more than one resource that matches the provided offering name '%s' and plan name '%s'. Please provide servicePlanID", serviceName, planName)
	}
	return "", err

}

// BuildURL builds the url with provided query parameters
func BuildURL(baseURL string, q *Parameters) string {
	queryParams := q.Encode()
	if queryParams == "" {
		return baseURL
	}
	return baseURL + "?" + queryParams
}

func ExtractInstanceID(operationURL string) string {
	r := regexp.MustCompile(`^/v1/service_instances/(.*)/operations/.*$`)
	matches := r.FindStringSubmatch(operationURL)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func ExtractBindingID(operationURL string) string {
	r := regexp.MustCompile(`^/v1/service_bindings/(.*)/operations/.*$`)
	matches := r.FindStringSubmatch(operationURL)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func BuildOperationURL(operationID, resourceID, resourceURL string) string {
	return fmt.Sprintf("%s/%s%s/%s", resourceURL, resourceID, web.ResourceOperationsURL, operationID)
}
