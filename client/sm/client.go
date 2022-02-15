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

	smtypes "github.com/Peripli/service-manager/pkg/types"

	"github.com/Peripli/service-manager/pkg/log"
	"github.com/Peripli/service-manager/pkg/util"
	"github.com/Peripli/service-manager/pkg/web"
	"github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/SAP/sap-btp-service-operator/internal/auth"
	"github.com/SAP/sap-btp-service-operator/internal/httputil"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	originatingIdentityHeader = "X-Originating-Identity"
)

// Client should be implemented by SM clients
//go:generate counterfeiter . Client
type Client interface {
	ListInstances(*Parameters) (*types.ServiceInstances, error)
	GetInstanceByID(string, *Parameters) (*types.ServiceInstance, error)
	UpdateInstance(id string, updatedInstance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string) (*types.ServiceInstance, string, error)
	Provision(instance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string) (*ProvisionResponse, error)
	Deprovision(id string, q *Parameters, user string) (string, error)

	ListBindings(*Parameters) (*types.ServiceBindings, error)
	GetBindingByID(string, *Parameters) (*types.ServiceBinding, error)
	Bind(binding *types.ServiceBinding, q *Parameters, user string) (*types.ServiceBinding, string, error)
	Unbind(id string, q *Parameters, user string) (string, error)
	RenameBinding(id, newName, newK8SName string) (*types.ServiceBinding, error)

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

type planInfo struct {
	planID          string
	serviceOffering *types.ServiceOffering
}

type ProvisionResponse struct {
	InstanceID string
	PlanID     string
	Location   string
	Tags       json.RawMessage
}

// NewClientWithAuth returns new SM Client configured with the provided configuration
func NewClient(ctx context.Context, config *ClientConfig, httpClient auth.HTTPClient) (Client, error) {
	if httpClient != nil {
		return &serviceManagerClient{Context: ctx, Config: config, HTTPClient: httpClient}, nil
	}
	ccConfig := &clientcredentials.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		TokenURL:     config.TokenURL + config.TokenURLSuffix,
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	var authClient auth.HTTPClient
	var err error
	if len(config.TLSCertKey) > 0 && len(config.TLSPrivateKey) > 0 {
		authClient, err = auth.NewAuthClientWithTLS(ccConfig, config.TLSCertKey, config.TLSPrivateKey)
		if err != nil {
			return nil, err
		}
	} else {
		authClient = auth.NewAuthClient(ccConfig, config.SSLDisabled)
	}
	return &serviceManagerClient{Context: ctx, Config: config, HTTPClient: authClient}, nil
}

// Provision provisions a new service instance in service manager
func (client *serviceManagerClient) Provision(instance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string) (*ProvisionResponse, error) {
	var newInstance *types.ServiceInstance
	var instanceID string
	if len(serviceName) == 0 || len(planName) == 0 {
		return nil, fmt.Errorf("missing field values. Specify service name and plan name for the instance '%s'", instance.Name)
	}

	planInfo, err := client.getPlanInfo(instance.ServicePlanID, serviceName, planName)
	if err != nil {
		return nil, err
	}

	instance.ServicePlanID = planInfo.planID

	location, err := client.register(instance, web.ServiceInstancesURL, q, user, &newInstance)
	if err != nil {
		return nil, err
	}
	if len(location) > 0 {
		instanceID = ExtractInstanceID(location)
		if len(instanceID) == 0 {
			return nil, fmt.Errorf("failed to extract the service instance ID from the async operation URL: %s", location)
		}
	} else if newInstance != nil {
		instanceID = newInstance.ID
	}

	res := &ProvisionResponse{
		InstanceID: instanceID,
		Location:   location,
		PlanID:     planInfo.planID,
	}
	if planInfo.serviceOffering != nil {
		res.Tags = planInfo.serviceOffering.Tags
	}
	return res, nil

}

// Bind creates binding to an instance in service manager
func (client *serviceManagerClient) Bind(binding *types.ServiceBinding, q *Parameters, user string) (*types.ServiceBinding, string, error) {
	var newBinding *types.ServiceBinding
	location, err := client.register(binding, web.ServiceBindingsURL, q, user, &newBinding)
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

func (client *serviceManagerClient) Deprovision(id string, q *Parameters, user string) (string, error) {
	return client.delete(web.ServiceInstancesURL+"/"+id, q, user)
}

func (client *serviceManagerClient) Unbind(id string, q *Parameters, user string) (string, error) {
	return client.delete(web.ServiceBindingsURL+"/"+id, q, user)
}

func (client *serviceManagerClient) UpdateInstance(id string, updatedInstance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string) (*types.ServiceInstance, string, error) {
	var result *types.ServiceInstance

	planInfo, err := client.getPlanInfo(updatedInstance.ServicePlanID, serviceName, planName)
	if err != nil {
		return nil, "", err
	}
	updatedInstance.ServicePlanID = planInfo.planID
	location, err := client.update(updatedInstance, web.ServiceInstancesURL, id, q, user, &result)
	if err != nil {
		return nil, "", err
	}
	return result, location, nil
}

func (client *serviceManagerClient) RenameBinding(id, newName, newK8SName string) (*types.ServiceBinding, error) {
	const k8sNameLabel = "_k8sname"
	renameRequest := map[string]interface{}{
		"name": newName,
		"labels": []*smtypes.LabelChange{
			{
				Key:       k8sNameLabel,
				Operation: smtypes.RemoveLabelOperation,
				Values:    []string{},
			},
			{
				Key:       k8sNameLabel,
				Operation: smtypes.AddLabelOperation,
				Values:    []string{newK8SName},
			},
		},
	}

	var result *types.ServiceBinding
	_, err := client.update(renameRequest, web.ServiceBindingsURL, id, nil, "", &result)
	if err != nil {
		return nil, err
	}
	return result, err
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

func (client *serviceManagerClient) register(resource interface{}, url string, q *Parameters, user string, result interface{}) (string, error) {
	requestBody, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}

	buffer := bytes.NewBuffer(requestBody)
	response, err := client.callWithUser(http.MethodPost, url, buffer, q, user)
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

func (client *serviceManagerClient) delete(url string, q *Parameters, user string) (string, error) {
	response, err := client.callWithUser(http.MethodDelete, url, nil, q, user)
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

func (client *serviceManagerClient) update(resource interface{}, url string, id string, q *Parameters, user string, result interface{}) (string, error) {
	requestBody, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}
	buffer := bytes.NewBuffer(requestBody)
	response, err := client.callWithUser(http.MethodPatch, url+"/"+id, buffer, q, user)
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
	return client.callWithUser(method, smpath, body, q, "")
}

func (client *serviceManagerClient) callWithUser(method string, smpath string, body io.Reader, q *Parameters, user string) (*http.Response, error) {
	fullURL := httputil.NormalizeURL(client.Config.URL) + BuildURL(smpath, q)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	if len(user) > 0 {
		req.Header.Add(originatingIdentityHeader, user)
	}

	log.C(client.Context).Debugf("Sending request %s %s", req.Method, req.URL)
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (client *serviceManagerClient) getPlanInfo(planID string, serviceName string, planName string) (*planInfo, error) {
	offerings, err := client.getServiceOfferingsByName(serviceName)
	if err != nil {
		return nil, err
	}

	var commaSepOfferingIds string
	if len(offerings.ServiceOfferings) == 0 {
		return nil, fmt.Errorf("couldn't find the service offering '%s'", serviceName)
	}

	serviceOfferingIds := make([]string, 0, len(offerings.ServiceOfferings))
	for _, svc := range offerings.ServiceOfferings {
		serviceOfferingIds = append(serviceOfferingIds, svc.ID)
	}
	commaSepOfferingIds = "'" + strings.Join(serviceOfferingIds, "', '") + "'"

	query := &Parameters{
		FieldQuery: []string{fmt.Sprintf("catalog_name eq '%s'", planName), fmt.Sprintf("service_offering_id in (%s)", commaSepOfferingIds)},
	}

	plans, err := client.ListPlans(query)
	if err != nil {
		return nil, err
	}
	if len(plans.ServicePlans) == 0 {
		return nil, fmt.Errorf("couldn't find the service plan '%s' for the service offering '%s'", planName, serviceName)
	} else if len(plans.ServicePlans) == 1 && len(planID) == 0 {
		return &planInfo{
			planID:          plans.ServicePlans[0].ID,
			serviceOffering: findOffering(plans.ServicePlans[0].ServiceOfferingID, offerings),
		}, nil
	} else {
		for _, plan := range plans.ServicePlans {
			if plan.ID == planID {
				return &planInfo{
					planID:          plan.ID,
					serviceOffering: findOffering(plan.ServiceOfferingID, offerings),
				}, nil
			}
		}
	}

	if len(planID) > 0 {
		err = fmt.Errorf("the provided plan ID '%s' doesn't match the provided offering name '%s' and plan name '%s", planID, serviceName, planName)
	} else {
		err = fmt.Errorf("ambiguity error: found more than one resource that matches the provided offering name '%s' and plan name '%s'. Please provide servicePlanID", serviceName, planName)
	}
	return nil, err
}

func (client *serviceManagerClient) getServiceOfferingsByName(serviceName string) (*types.ServiceOfferings, error) {
	query := &Parameters{
		FieldQuery: []string{fmt.Sprintf("catalog_name eq '%s'", serviceName)},
	}
	offerings, err := client.ListOfferings(query)
	if err != nil {
		return nil, err
	}
	return offerings, nil
}

func findOffering(id string, offerings *types.ServiceOfferings) *types.ServiceOffering {
	for _, off := range offerings.ServiceOfferings {
		off := off
		if off.ID == id {
			return &off
		}
	}
	return nil
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
