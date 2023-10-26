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
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/SAP/sap-btp-service-operator/api"
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
//
//go:generate counterfeiter . Client
type Client interface {
	ListInstances(*Parameters) (*types.ServiceInstances, error)
	GetInstanceByID(string, *Parameters) (*types.ServiceInstance, error)
	UpdateInstance(id string, updatedInstance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string, dataCenter string) (*types.ServiceInstance, string, error)
	Provision(instance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string, dataCenter string) (*ProvisionResponse, error)
	Deprovision(id string, q *Parameters, user string) (string, error)

	ListBindings(*Parameters) (*types.ServiceBindings, error)
	GetBindingByID(string, *Parameters) (*types.ServiceBinding, error)
	Bind(binding *types.ServiceBinding, q *Parameters, user string) (*types.ServiceBinding, string, error)
	Unbind(id string, q *Parameters, user string) (string, error)
	RenameBinding(id, newName, newK8SName string) (*types.ServiceBinding, error)
	ShareInstance(id string, user string) error
	UnShareInstance(id string, user string) error

	ListOfferings(*Parameters) (*types.ServiceOfferings, error)
	ListPlans(*Parameters) (*types.ServicePlans, error)

	Status(string, *Parameters) (*types.Operation, error)

	// Call makes HTTP request to the Service Manager server with authentication.
	// It should be used only in case there is no already implemented method for such an operation
	Call(method string, smpath string, body io.Reader, q *Parameters) (*http.Response, error)
}

type ServiceManagerError struct {
	ErrorType   string                   `json:"error,omitempty"`
	Description string                   `json:"description,omitempty"`
	StatusCode  int                      `json:"-"`
	BrokerError *api.HTTPStatusCodeError `json:"broker_error,omitempty"`
}

func (e *ServiceManagerError) Error() string {
	if e.BrokerError != nil {
		return e.BrokerError.Error()
	}

	return e.Description
}

func (e *ServiceManagerError) GetStatusCode() int {
	if e.BrokerError != nil {
		return e.BrokerError.StatusCode
	}

	return e.StatusCode
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
	InstanceID   string
	PlanID       string
	Location     string
	SubaccountID string
	Tags         json.RawMessage
}

// NewClient NewClientWithAuth returns new SM Client configured with the provided configuration
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
func (client *serviceManagerClient) Provision(instance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string, dataCenter string) (*ProvisionResponse, error) {
	var newInstance *types.ServiceInstance
	var instanceID string
	var subaccountID string

	if len(serviceName) == 0 || len(planName) == 0 {
		return nil, fmt.Errorf("missing field values. Specify service name and plan name for the instance '%s'", instance.Name)
	}

	planInfo, err := client.getPlanInfo(instance.ServicePlanID, serviceName, planName, dataCenter)
	if err != nil {
		return nil, err
	}

	instance.ServicePlanID = planInfo.planID

	location, err := client.register(instance, types.ServiceInstancesURL, q, user, &newInstance)
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
		subaccountID = getSubaccountID(newInstance)
	}

	res := &ProvisionResponse{
		InstanceID:   instanceID,
		Location:     location,
		PlanID:       planInfo.planID,
		SubaccountID: subaccountID,
	}

	if planInfo.serviceOffering != nil {
		res.Tags = planInfo.serviceOffering.Tags
	}

	return res, nil
}

// Bind creates binding to an instance in service manager
func (client *serviceManagerClient) Bind(binding *types.ServiceBinding, q *Parameters, user string) (*types.ServiceBinding, string, error) {
	var newBinding *types.ServiceBinding
	location, err := client.register(binding, types.ServiceBindingsURL, q, user, &newBinding)
	if err != nil {
		return nil, "", err
	}
	return newBinding, location, nil
}

// ListInstances returns service instances registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) ListInstances(q *Parameters) (*types.ServiceInstances, error) {
	instances := &types.ServiceInstances{}
	err := client.list(&instances.ServiceInstances, types.ServiceInstancesURL, q)

	return instances, err
}

// GetInstanceByID returns instance registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) GetInstanceByID(id string, q *Parameters) (*types.ServiceInstance, error) {
	instance := &types.ServiceInstance{}
	err := client.get(instance, types.ServiceInstancesURL+"/"+id, q)

	return instance, err
}

// ListBindings returns service bindings registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) ListBindings(q *Parameters) (*types.ServiceBindings, error) {
	bindings := &types.ServiceBindings{}
	err := client.list(&bindings.ServiceBindings, types.ServiceBindingsURL, q)

	return bindings, err
}

// GetBindingByID returns binding registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) GetBindingByID(id string, q *Parameters) (*types.ServiceBinding, error) {
	binding := &types.ServiceBinding{}
	err := client.get(binding, types.ServiceBindingsURL+"/"+id, q)

	return binding, err
}

func (client *serviceManagerClient) Status(url string, q *Parameters) (*types.Operation, error) {
	operation := &types.Operation{}
	err := client.get(operation, url, q)

	return operation, err
}

func (client *serviceManagerClient) Deprovision(id string, q *Parameters, user string) (string, error) {
	return client.delete(types.ServiceInstancesURL+"/"+id, q, user)
}

func (client *serviceManagerClient) Unbind(id string, q *Parameters, user string) (string, error) {
	return client.delete(types.ServiceBindingsURL+"/"+id, q, user)
}

func (client *serviceManagerClient) UpdateInstance(id string, updatedInstance *types.ServiceInstance, serviceName string, planName string, q *Parameters, user string, dataCenter string) (*types.ServiceInstance, string, error) {
	var result *types.ServiceInstance

	planInfo, err := client.getPlanInfo(updatedInstance.ServicePlanID, serviceName, planName, dataCenter)
	if err != nil {
		return nil, "", err
	}
	updatedInstance.ServicePlanID = planInfo.planID
	location, err := client.update(updatedInstance, types.ServiceInstancesURL, id, q, user, &result)
	if err != nil {
		return nil, "", err
	}
	return result, location, nil
}

func (client *serviceManagerClient) RenameBinding(id, newName, newK8SName string) (*types.ServiceBinding, error) {
	const k8sNameLabel = "_k8sname"
	renameRequest := map[string]interface{}{
		"name": newName,
		"labels": []*types.LabelChange{
			{
				Key:       k8sNameLabel,
				Operation: types.RemoveLabelOperation,
			},
		},
	}

	var result *types.ServiceBinding
	_, err := client.update(renameRequest, types.ServiceBindingsURL, id, nil, "", &result)
	if err != nil {
		return nil, err
	}

	// due to sm issue (no "replace" label and remove+add not supported in the same request)
	// adding the new _k8sname value in another request
	addLabelRequest := map[string]interface{}{
		"labels": []*types.LabelChange{
			{
				Key:       k8sNameLabel,
				Operation: types.AddLabelOperation,
				Values:    []string{newK8SName},
			},
		},
	}
	_, err = client.update(addLabelRequest, types.ServiceBindingsURL, id, nil, "", &result)
	if err != nil {
		return nil, err
	}
	return result, err
}

func (client *serviceManagerClient) list(items interface{}, url string, q *Parameters) error {
	itemsType := reflect.TypeOf(items)
	if itemsType.Kind() != reflect.Ptr || itemsType.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("items should be a pointer to a slice, but got %v", itemsType)
	}

	allItems := reflect.MakeSlice(itemsType.Elem(), 0, 0)
	iter := listIterator{
		URL:    url,
		Params: q,
		Client: client,
	}

	more := true
	for more {
		var err error
		pageSlice := reflect.New(itemsType.Elem())
		more, _, err = iter.nextPage(pageSlice.Interface(), -1)
		if err != nil {
			return err
		}
		allItems = reflect.AppendSlice(allItems, pageSlice.Elem())
	}
	reflect.ValueOf(items).Elem().Set(allItems)
	return nil
}

func (client *serviceManagerClient) ListOfferings(q *Parameters) (*types.ServiceOfferings, error) {
	offerings := &types.ServiceOfferings{}
	err := client.list(&offerings.ServiceOfferings, types.ServiceOfferingsURL, q)

	return offerings, err
}

func (client *serviceManagerClient) ListPlans(q *Parameters) (*types.ServicePlans, error) {
	plans := &types.ServicePlans{}
	err := client.list(&plans.ServicePlans, types.ServicePlansURL, q)

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
		return "", handleResponseError(response)
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
		return "", handleResponseError(response)
	}
}

func (client *serviceManagerClient) get(result interface{}, url string, q *Parameters) error {
	response, err := client.Call(http.MethodGet, url, nil, q)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return handleResponseError(response)
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
		return "", handleResponseError(response)
	}
}

func (client *serviceManagerClient) ShareInstance(id string, user string) error {
	return client.executeShareInstanceRequest(true, id, user)
}

func (client *serviceManagerClient) UnShareInstance(id string, user string) error {
	return client.executeShareInstanceRequest(false, id, user)
}

func (client *serviceManagerClient) executeShareInstanceRequest(shouldShare bool, id string, user string) error {
	bodyRequest := map[string]interface{}{
		"shared": shouldShare,
	}
	shareBody, err := json.Marshal(bodyRequest)
	if err != nil {
		return err
	}

	buffer := bytes.NewBuffer(shareBody)

	response, err := client.callWithUser(http.MethodPatch, types.ServiceInstancesURL+"/"+id, buffer, nil, user)
	if response.StatusCode != http.StatusOK {
		if err == nil {
			return handleResponseError(response)
		}
		return httputil.UnmarshalResponse(response, err)
	}

	return nil
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

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (client *serviceManagerClient) getPlanInfo(planID string, serviceName string, planName string, dataCenter string) (*planInfo, error) {

	offerings, err := client.getServiceOfferingsByNameAndDataCenter(serviceName, dataCenter)
	if err != nil {
		return nil, err
	}

	var commaSepOfferingIds string
	if len(offerings.ServiceOfferings) == 0 {
		return nil, fmt.Errorf("couldn't find the service offering '%s' on dataCenter '%s'", serviceName, dataCenter)
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

func (client *serviceManagerClient) getServiceOfferingsByNameAndDataCenter(serviceName string, dataCenter string) (*types.ServiceOfferings, error) {
	query := &Parameters{
		FieldQuery: []string{fmt.Sprintf("catalog_name eq '%s' and data_center eq '%s'", serviceName, dataCenter)},
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

func getSubaccountID(instance *types.ServiceInstance) string {
	if len(instance.Labels["subaccount_id"]) > 0 {
		return instance.Labels["subaccount_id"][0]
	}
	subaccountID := ""
	if instance == nil || len(instance.Context) == 0 {
		return subaccountID
	}

	var contextMap map[string]interface{}
	if err := json.Unmarshal(instance.Context, &contextMap); err != nil {
		return ""
	}

	if saID, ok := contextMap["subaccount_id"]; ok {
		subaccountID, _ = saID.(string)
	}
	return subaccountID
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
	return fmt.Sprintf("%s/%s%s/%s", resourceURL, resourceID, types.ResourceOperationsURL, operationID)
}

func handleResponseError(response *http.Response) error {
	body, err := bodyToBytes(response.Body)
	if err != nil {
		body = []byte(fmt.Sprintf("error reading response body: %s", err))
	}

	smError := &ServiceManagerError{
		StatusCode: response.StatusCode,
	}
	_ = json.Unmarshal(body, &smError)

	return smError
}

func bodyToBytes(closer io.ReadCloser) ([]byte, error) {
	defer closer.Close()

	body, err := io.ReadAll(closer)
	if err != nil {
		return nil, err
	}
	return body, nil
}

type listIterator struct {
	Client
	URL    string
	Params *Parameters
	next   string
	done   bool
}

type listResponse struct {
	Token    string      `json:"token"`
	NumItems int64       `json:"num_items"`
	Items    interface{} `json:"items"`
}

func (li *listIterator) nextPage(items interface{}, maxItems int) (more bool, count int64, err error) {
	itemsType := reflect.TypeOf(items)
	if itemsType != nil && (itemsType.Kind() != reflect.Ptr || itemsType.Elem().Kind() != reflect.Slice) {
		return false, -1, fmt.Errorf("items should be nil or a pointer to a slice, but got %v", itemsType)
	}

	if li.done {
		return false, -1, fmt.Errorf("iteration already complete")
	}

	if li.Params == nil {
		li.Params = &Parameters{}
	}
	if maxItems >= 0 {
		li.Params.GeneralParams = append(li.Params.GeneralParams, fmt.Sprintf("max_items=%s", strconv.Itoa(maxItems)))
	}
	if li.next != "" {
		li.Params.GeneralParams = append(li.Params.GeneralParams, fmt.Sprintf("token=%s", li.next))
	}

	method := http.MethodGet
	url := li.URL
	response, err := li.Call(method, url, nil, li.Params)
	if err != nil {
		return false, -1, fmt.Errorf("error sending request %s %s: %s", method, url, err)
	}
	if response.Request != nil {
		url = response.Request.URL.String() // should include also the query params
	}
	if response.StatusCode != http.StatusOK {
		return false, -1, handleResponseError(response)
	}
	body, err := bodyToBytes(response.Body)
	if err != nil {
		return false, -1, fmt.Errorf("error reading response body of request %s %s: %s",
			method, url, err)
	}
	responseBody := listResponse{Items: items}
	if err = json.Unmarshal(body, &responseBody); err != nil {
		return false, -1, fmt.Errorf("error parsing response body of request %s %s: %s",
			method, url, err)
	}

	li.next = responseBody.Token
	li.done = li.next == ""
	return !li.done, responseBody.NumItems, nil
}
