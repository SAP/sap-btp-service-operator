package sm

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/SAP/sap-btp-service-operator/client/sm/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client test", func() {

	Describe("Instances", func() {
		var (
			instance    *types.ServiceInstance
			serviceID   = "service_id"
			serviceName = "mongo"
			planName    = "small"
			planID      = "service_plan_id"
		)

		BeforeEach(func() {
			instance = &types.ServiceInstance{
				ID:            "instanceID",
				Name:          "instance1",
				ServicePlanID: planID,
				PlatformID:    "platform_id",
				Context:       json.RawMessage("{}"),
			}
			instancesArray := []types.ServiceInstance{*instance}
			instances := types.ServiceInstances{ServiceInstances: instancesArray}
			responseBody, _ := json.Marshal(instances)

			handlerDetails = []HandlerDetails{
				{Method: http.MethodGet, Path: types.ServiceInstancesURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
			}
		})

		Describe("List service instances", func() {
			Context("When there are service instances registered", func() {
				It("should return all", func() {
					result, err := client.ListInstances(params)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(result.ServiceInstances).To(HaveLen(1))
					Expect(result.ServiceInstances[0]).To(Equal(*instance))
				})
			})

			Context("When there are no service instances registered", func() {
				BeforeEach(func() {
					var instancesArray []types.ServiceInstance
					instances := types.ServiceInstances{ServiceInstances: instancesArray}
					responseBody, _ := json.Marshal(instances)

					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
					}
				})
				It("should return an empty array", func() {
					result, err := client.ListInstances(params)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(result.ServiceInstances).To(HaveLen(0))
				})
			})

			Context("When invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL, ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should handle status code != 200", func() {
					_, err := client.ListInstances(params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusCreated)
				})
			})

			Context("When invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL, ResponseStatusCode: http.StatusBadRequest},
					}
				})
				It("should handle status code > 299", func() {
					_, err := client.ListInstances(params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusBadRequest)
				})
			})
		})

		Describe("Get service instance", func() {
			Context("When there is instance with this id", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(instance)
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
					}
				})
				It("should return it", func() {
					result, err := client.GetInstanceByID(instance.ID, params)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(result).To(Equal(instance))
				})
			})

			Context("When there is no instance with this id", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL + "/", ResponseStatusCode: http.StatusNotFound},
					}
				})
				It("should return 404", func() {
					_, err := client.GetInstanceByID(instance.ID, params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusNotFound)
				})
			})

			Context("When invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL + "/", ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should handle status code != 200", func() {
					_, err := client.GetInstanceByID(instance.ID, params)
					Expect(err).Should(HaveOccurred())
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusCreated)
				})
			})

			Context("When invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL + "/", ResponseStatusCode: http.StatusBadRequest},
					}
				})

				It("should handle status code > 299", func() {
					_, err := client.GetInstanceByID(instance.ID, params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusBadRequest)
				})
			})
		})

		Describe("Provision", func() {
			BeforeEach(func() {
				instanceResponseBody, _ := json.Marshal(instance)
				offering := &types.ServiceOffering{
					ID:          "service_id",
					Name:        serviceName,
					CatalogName: serviceName,
				}
				plan := &types.ServicePlan{
					ID:                planID,
					Name:              planName,
					CatalogName:       serviceName,
					ServiceOfferingID: serviceID,
				}

				plansArray := []types.ServicePlan{*plan}
				plans := types.ServicePlans{ServicePlans: plansArray}
				plansBody, _ := json.Marshal(plans)

				offeringArray := []types.ServiceOffering{*offering}
				offerings := types.ServiceOfferings{ServiceOfferings: offeringArray}
				offeringResponseBody, _ := json.Marshal(offerings)
				handlerDetails = []HandlerDetails{
					{Method: http.MethodPost, Path: types.ServiceInstancesURL, ResponseBody: instanceResponseBody, ResponseStatusCode: http.StatusCreated},
					{Method: http.MethodGet, Path: types.ServiceOfferingsURL, ResponseBody: offeringResponseBody, ResponseStatusCode: http.StatusOK},
					{Method: http.MethodGet, Path: types.ServicePlansURL, ResponseBody: plansBody, ResponseStatusCode: http.StatusOK},
				}
			})

			Context("When valid instance is being provisioned synchronously", func() {
				It("should provision successfully", func() {
					res, err := client.Provision(instance, serviceName, planName, params, "test-user", "")

					Expect(err).ShouldNot(HaveOccurred())
					Expect(res.Location).Should(HaveLen(0))
					Expect(res.InstanceID).To(Equal(instance.ID))
				})

				Context("When multiple matching plan names returned from SM", func() {
					BeforeEach(func() {
						plan1 := &types.ServicePlan{
							ID:                planID,
							Name:              planName,
							CatalogName:       serviceName,
							ServiceOfferingID: serviceID,
						}
						plan2 := &types.ServicePlan{
							ID:                "planID2",
							Name:              planName,
							CatalogName:       serviceName,
							ServiceOfferingID: serviceID,
						}

						plansArray := []types.ServicePlan{*plan1, *plan2}
						plans := types.ServicePlans{ServicePlans: plansArray}
						plansBody, _ := json.Marshal(plans)
						handlerDetails[2] = HandlerDetails{Method: http.MethodGet, Path: types.ServicePlansURL, ResponseBody: plansBody, ResponseStatusCode: http.StatusOK}
					})

					It("should provision successfully", func() {
						res, err := client.Provision(instance, "mongo", "small", params, "test-user", "")
						Expect(err).ShouldNot(HaveOccurred())
						Expect(res.Location).Should(HaveLen(0))
						Expect(res.InstanceID).To(Equal(instance.ID))
					})
				})

				Context("When no plan id provided", func() {
					It("should provision successfully", func() {
						instance.ServicePlanID = ""
						res, err := client.Provision(instance, "mongo", "small", params, "test-user", "")
						Expect(err).ShouldNot(HaveOccurred())
						Expect(res.Location).Should(HaveLen(0))
						Expect(res.InstanceID).To(Equal(instance.ID))
					})
				})
			})

			Context("When invalid instance is being provisioned synchronously", func() {
				When("No service name", func() {
					It("should fail to provision", func() {
						res, err := client.Provision(instance, "", "small", params, "test-user", "")
						Expect(err).Should(HaveOccurred())
						Expect(res).To(BeNil())
					})
				})

				When("No plan name", func() {
					It("should fail to provision", func() {
						res, err := client.Provision(instance, "mongo", "", params, "test-user", "")
						Expect(err).Should(HaveOccurred())
						Expect(res).To(BeNil())
					})
				})

				When("Plan id not match plan name", func() {
					It("should fail", func() {
						instance.ServicePlanID = "some-id"
						res, err := client.Provision(instance, "mongo", "small", params, "test-user", "")
						Expect(err).Should(HaveOccurred())
						Expect(res).To(BeNil())
					})
				})

				When("Service not exist", func() {
					BeforeEach(func() {
						var offeringsArray []types.ServiceOffering
						offerings := types.ServiceOfferings{ServiceOfferings: offeringsArray}
						responseBody, _ := json.Marshal(offerings)
						handlerDetails[1] = HandlerDetails{Method: http.MethodGet, Path: types.ServiceOfferingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK}
					})
					It("should fail", func() {
						res, err := client.Provision(instance, "mongo2", "small", params, "test-user", "")
						Expect(err).Should(HaveOccurred())
						Expect(res).To(BeNil())
					})
				})
			})

			Context("When valid instance is being provisioned asynchronously", func() {
				var locationHeader string
				BeforeEach(func() {
					locationHeader = "/v1/service_instances/12345/operations/1234"
					handlerDetails[0] = HandlerDetails{Method: http.MethodPost, Path: types.ServiceInstancesURL, ResponseStatusCode: http.StatusAccepted, Headers: map[string]string{"Location": locationHeader}}
				})
				It("should receive operation location", func() {
					res, err := client.Provision(instance, serviceName, planName, params, "test-user", "")

					Expect(err).ShouldNot(HaveOccurred())
					Expect(res.Location).Should(Equal(locationHeader))
					Expect(res.InstanceID).Should(Equal("12345"))
				})
			})

			Context("When invalid instance is being returned by SM", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(struct {
						Name bool `json:"name"`
					}{
						Name: true,
					})
					handlerDetails[0] = HandlerDetails{Method: http.MethodPost, Path: types.ServiceInstancesURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusCreated}

				})
				It("should return error", func() {
					res, err := client.Provision(instance, serviceName, planName, params, "test-user", "")

					Expect(err).Should(HaveOccurred())
					Expect(res).To(BeNil())
				})
			})

			Context("When invalid status code is returned by SM", func() {
				Context("And status code is unsuccessful", func() {
					BeforeEach(func() {
						responseBody, _ := json.Marshal(instance)
						handlerDetails[0] = HandlerDetails{Method: http.MethodPost, Path: types.ServiceInstancesURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK}
					})
					It("should return error with status code", func() {
						res, err := client.Provision(instance, serviceName, planName, params, "test-user", "")
						expectErrorToContainSubstringAndStatusCode(err, "", http.StatusOK)
						Expect(res).To(BeNil())
					})
				})

				Context("And status code is unsuccessful", func() {
					BeforeEach(func() {
						responseBody := []byte(`{ "description": "description"}`)
						handlerDetails[0] = HandlerDetails{Method: http.MethodPost, Path: types.ServiceInstancesURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusBadRequest}

					})
					It("should return error with url and description", func() {
						res, err := client.Provision(instance, serviceName, planName, params, "test-user", "")
						expectErrorToContainSubstringAndStatusCode(err, "description", http.StatusBadRequest)
						Expect(res).To(BeNil())
					})
				})

				Context("And invalid response body", func() {
					BeforeEach(func() {
						responseBody := []byte(`{ "description": description" }`)
						handlerDetails[0] = HandlerDetails{Method: http.MethodPost, Path: types.ServiceInstancesURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusBadRequest}

					})
					It("should return error without url and description if invalid response body", func() {
						res, err := client.Provision(instance, serviceName, planName, params, "test-user", "")
						expectErrorToContainSubstringAndStatusCode(err, "", http.StatusBadRequest)
						Expect(res).To(BeNil())
					})
				})

			})

		})

		Describe("Deprovision", func() {
			Context("when an existing instance is being deleted synchronously", func() {
				BeforeEach(func() {
					responseBody := []byte("{}")
					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceInstancesURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
					}
				})
				It("should be successfully removed", func() {
					location, err := client.Deprovision(instance.ID, params, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(BeEmpty())
				})
			})

			Context("when an existing instance is being deleted asynchronously", func() {
				var locationHeader string
				BeforeEach(func() {
					locationHeader = "location"
					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceInstancesURL + "/", ResponseStatusCode: http.StatusAccepted, Headers: map[string]string{"Location": locationHeader}},
					}
				})
				It("should be successfully removed", func() {
					location, err := client.Deprovision(instance.ID, params, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(Equal(locationHeader))
				})
			})

			Context("when service manager returns a non-expected status code", func() {
				BeforeEach(func() {
					responseBody := []byte("{}")

					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceInstancesURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should handle error", func() {
					location, err := client.Deprovision(instance.ID, params, "test-user")
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusCreated)
					Expect(location).Should(BeEmpty())
				})
			})

			Context("when service manager returns a status code not found", func() {
				BeforeEach(func() {
					responseBody := []byte(`{ "description": "Instance not found" }`)

					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceInstancesURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusNotFound},
					}
				})
				It("should be considered as success", func() {
					location, err := client.Deprovision(instance.ID, params, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(BeEmpty())
				})
			})
		})

		Describe("Update", func() {
			BeforeEach(func() {
				offering := &types.ServiceOffering{
					ID:          "service_id",
					Name:        serviceName,
					CatalogName: serviceName,
				}
				plan := &types.ServicePlan{
					ID:                planID,
					Name:              planName,
					CatalogName:       serviceName,
					ServiceOfferingID: serviceID,
				}

				plansArray := []types.ServicePlan{*plan}
				plans := types.ServicePlans{ServicePlans: plansArray}
				plansBody, _ := json.Marshal(plans)
				offeringArray := []types.ServiceOffering{*offering}
				offerings := types.ServiceOfferings{ServiceOfferings: offeringArray}
				offeringResponseBody, _ := json.Marshal(offerings)
				handlerDetails = []HandlerDetails{
					{Method: http.MethodGet, Path: types.ServiceOfferingsURL, ResponseBody: offeringResponseBody, ResponseStatusCode: http.StatusOK},
					{Method: http.MethodGet, Path: types.ServicePlansURL, ResponseBody: plansBody, ResponseStatusCode: http.StatusOK},
				}
			})
			Context("When valid instance is being updated synchronously", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(instance)
					handlerDetails = append(handlerDetails,
						HandlerDetails{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK})
				})
				It("should update successfully", func() {
					responseInstance, location, err := client.UpdateInstance(instance.ID, instance, serviceName, planName, params, "test-user", "")

					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(HaveLen(0))
					Expect(responseInstance).To(Equal(instance))
				})
			})

			Context("When valid instance is being updated asynchronously", func() {
				var locationHeader string
				BeforeEach(func() {
					locationHeader = "test-location"
					handlerDetails = append(handlerDetails,
						HandlerDetails{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseStatusCode: http.StatusAccepted, Headers: map[string]string{"Location": locationHeader}})
				})
				It("should receive operation location", func() {
					responseInstance, location, err := client.UpdateInstance(instance.ID, instance, serviceName, planName, params, "test-user", "")

					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(Equal(locationHeader))
					Expect(responseInstance).To(BeNil())
				})
			})

			Context("When invalid instance is being returned by SM", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(struct {
						Name bool `json:"name"`
					}{
						Name: true,
					})
					handlerDetails = append(handlerDetails,
						HandlerDetails{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK})
				})
				It("should return error", func() {
					responseInstance, location, err := client.UpdateInstance(instance.ID, instance, serviceName, planName, params, "test-user", "")

					Expect(err).Should(HaveOccurred())
					Expect(location).Should(BeEmpty())
					Expect(responseInstance).To(BeNil())
				})
			})

			Context("When invalid status code is returned by SM", func() {
				Context("And status code is unsuccessful", func() {
					BeforeEach(func() {
						responseBody, _ := json.Marshal(instance)
						handlerDetails = append(handlerDetails,
							HandlerDetails{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusTeapot})
					})
					It("should return error with status code", func() {
						responseInstance, location, err := client.UpdateInstance(instance.ID, instance, serviceName, planName, params, "test-user", "")
						expectErrorToContainSubstringAndStatusCode(err, "", http.StatusTeapot)
						Expect(location).Should(BeEmpty())
						Expect(responseInstance).To(BeNil())
					})
				})

				Context("And status code is unsuccessful", func() {
					BeforeEach(func() {
						responseBody := []byte(`{ "description": "description"}`)
						handlerDetails = append(handlerDetails,
							HandlerDetails{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusBadRequest})
					})
					It("should return error with url and description", func() {
						responseInstance, location, err := client.UpdateInstance(instance.ID, instance, serviceName, planName, params, "test-user", "")
						expectErrorToContainSubstringAndStatusCode(err, "description", http.StatusBadRequest)
						Expect(location).Should(BeEmpty())
						Expect(responseInstance).To(BeNil())
					})
				})

				Context("And invalid response body", func() {
					BeforeEach(func() {
						responseBody := []byte(`{ "description": "description"}`)
						handlerDetails = append(handlerDetails,
							HandlerDetails{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusBadRequest})
					})
					It("should return error without url and description if invalid response body", func() {
						responseInstance, location, err := client.UpdateInstance(instance.ID, instance, serviceName, planName, params, "test-user", "")
						expectErrorToContainSubstringAndStatusCode(err, "description", http.StatusBadRequest)
						Expect(responseInstance).To(BeNil())
						Expect(location).Should(BeEmpty())
					})
				})

			})

			Context("When invalid config is set", func() {
				It("should return error", func() {
					var err error
					client, err = NewClient(context.TODO(), &ClientConfig{URL: "invalidURL"}, fakeAuthClient)
					Expect(err).ToNot(HaveOccurred())
					_, location, err := client.UpdateInstance(instance.ID, instance, serviceName, planName, params, "test-user", "")

					Expect(err).Should(HaveOccurred())
					Expect(location).Should(BeEmpty())
				})
			})
		})
	})

	Describe("Bindings", func() {
		var binding *types.ServiceBinding

		BeforeEach(func() {
			binding = &types.ServiceBinding{
				ID:                  "bindingID",
				Name:                "binding11",
				ServiceInstanceName: "instance1",
				Context:             json.RawMessage("{}"),
			}
		})

		It("validate binding id extraction from operation url", func() {
			bid := ExtractBindingID("/v1/service_bindings/1234/operations/5678")
			Expect(bid).To(Equal("1234"))
		})

		Describe("List service bindings", func() {
			Context("when there are service bindings registered", func() {
				BeforeEach(func() {
					binding = &types.ServiceBinding{
						ID:                "instanceID",
						Name:              "instance1",
						ServiceInstanceID: "service_instance_id",
					}
					bindingsArray := []types.ServiceBinding{*binding}
					bindings := types.ServiceBindings{ServiceBindings: bindingsArray}
					responseBody, _ := json.Marshal(bindings)

					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
					}
				})
				It("should return all", func() {
					result, err := client.ListBindings(params)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(result.ServiceBindings).To(HaveLen(1))
					Expect(result.ServiceBindings[0]).To(Equal(*binding))
				})
			})

			Context("when there are no service bindings registered", func() {
				BeforeEach(func() {
					var bindingsArray []types.ServiceBinding
					bindings := types.ServiceBindings{ServiceBindings: bindingsArray}
					responseBody, _ := json.Marshal(bindings)

					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
					}
				})
				It("should return an empty array", func() {
					result, err := client.ListBindings(params)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(result.ServiceBindings).To(HaveLen(0))
				})
			})

			Context("when invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL, ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should handle status code != 200", func() {
					_, err := client.ListBindings(params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusCreated)
				})
			})

			Context("when invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL, ResponseStatusCode: http.StatusBadRequest},
					}
				})
				It("should handle status code > 299", func() {
					_, err := client.ListBindings(params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusBadRequest)
				})
			})
		})

		Describe("Get service binding", func() {
			Context("when there is binding with this id", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(binding)
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
					}
				})
				It("should return it", func() {
					result, err := client.GetBindingByID(binding.ID, params)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(result).To(Equal(binding))
				})
			})

			Context("when there is no binding with this id", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL + "/", ResponseStatusCode: http.StatusNotFound},
					}
				})
				It("should return 404", func() {
					_, err := client.GetBindingByID(binding.ID, params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusNotFound)
				})
			})

			Context("when invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL + "/", ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should handle status code != 200", func() {
					_, err := client.GetBindingByID(binding.ID, params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusCreated)
				})
			})

			Context("when invalid status code is returned", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceBindingsURL + "/", ResponseStatusCode: http.StatusBadRequest},
					}
				})
				It("should handle status code > 299", func() {
					_, err := client.GetBindingByID(binding.ID, params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusBadRequest)

				})
			})
		})

		Describe("Bind", func() {
			Context("When valid binding is being created synchronously", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(binding)
					handlerDetails = []HandlerDetails{
						{Method: http.MethodPost, Path: types.ServiceBindingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should provision successfully", func() {
					responseBinding, location, err := client.Bind(binding, params, "test-user")

					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(HaveLen(0))
					Expect(responseBinding).To(Equal(binding))
				})
			})

			Context("When valid binding is being created asynchronously", func() {
				var locationHeader string
				BeforeEach(func() {
					locationHeader = "test-location"
					handlerDetails = []HandlerDetails{
						{Method: http.MethodPost, Path: types.ServiceBindingsURL, ResponseStatusCode: http.StatusAccepted, Headers: map[string]string{"Location": locationHeader}},
					}
				})
				It("should receive operation location", func() {
					responseBinding, location, err := client.Bind(binding, params, "test-user")

					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(Equal(locationHeader))
					Expect(responseBinding).To(BeNil())
				})
			})

			Context("When invalid binding is being returned by SM", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(struct {
						Name bool `json:"name"`
					}{
						Name: true,
					})
					handlerDetails = []HandlerDetails{
						{Method: http.MethodPost, Path: types.ServiceBindingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should return error", func() {
					responseBinding, location, err := client.Bind(binding, params, "test-user")

					Expect(err).Should(HaveOccurred())
					Expect(location).Should(BeEmpty())
					Expect(responseBinding).To(BeNil())
				})
			})

			Context("When invalid status code is returned by SM", func() {
				Context("And status code is unsuccessful", func() {
					BeforeEach(func() {
						responseBody, _ := json.Marshal(binding)
						handlerDetails = []HandlerDetails{
							{Method: http.MethodPost, Path: types.ServiceBindingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
						}
					})
					It("should return error with status code", func() {
						responseBinding, location, err := client.Bind(binding, params, "test-user")
						expectErrorToContainSubstringAndStatusCode(err, "", http.StatusOK)
						Expect(responseBinding).To(BeNil())
						Expect(location).Should(BeEmpty())
					})
				})

				Context("And status code is unsuccessful", func() {
					BeforeEach(func() {
						responseBody := []byte(`{ "description": "description"}`)
						handlerDetails = []HandlerDetails{
							{Method: http.MethodPost, Path: types.ServiceBindingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusBadRequest},
						}
					})
					It("should return error with url and description", func() {
						responseBinding, location, err := client.Bind(binding, params, "test-user")
						expectErrorToContainSubstringAndStatusCode(err, "description", http.StatusBadRequest)
						Expect(responseBinding).To(BeNil())
						Expect(location).Should(BeEmpty())
					})
				})

				Context("And invalid response body", func() {
					BeforeEach(func() {
						responseBody := []byte(`{ "description": description" }`)
						handlerDetails = []HandlerDetails{
							{Method: http.MethodPost, Path: types.ServiceBindingsURL, ResponseBody: responseBody, ResponseStatusCode: http.StatusBadRequest},
						}
					})
					It("should return error without url and description if invalid response body", func() {
						responseBinding, location, err := client.Bind(binding, params, "test-user")
						expectErrorToContainSubstringAndStatusCode(err, "", http.StatusBadRequest)
						Expect(responseBinding).To(BeNil())
						Expect(location).Should(BeEmpty())
					})
				})

			})

			Context("When invalid config is set", func() {
				It("should return error", func() {
					var err error
					client, err = NewClient(context.TODO(), &ClientConfig{URL: "invalidURL"}, fakeAuthClient)
					Expect(err).ToNot(HaveOccurred())
					_, location, err := client.Bind(binding, params, "test-user")

					Expect(err).Should(HaveOccurred())
					Expect(location).Should(BeEmpty())
				})
			})

			Context("When tls config is set", func() {
				var certificate, key string
				BeforeEach(func() {
					certificate = `-----BEGIN CERTIFICATE-----
MIIEVjCCAz6gAwIBAgIJAPgMH+mOpz2AMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAlVTMQswCQYDVQQIDAJOWTELMAkGA1UEBwwCTlkxDDAKBgNVBAoMA1NBUDEM
MAoGA1UECwwDU0FQMSUwIwYJKoZIhvcNAQkBFhZtYXlhLnNpZ2FsQGV4YW1wbGUu
Y29tMRIwEAYDVQQDDAlsb2NhbGhvc3QwHhcNMjEwNTIyMjMxNjI3WhcNMjIxMDA0
MjMxNjI3WjB+MQswCQYDVQQGEwJVUzELMAkGA1UECAwCTlkxCzAJBgNVBAcMAk5Z
MQwwCgYDVQQKDANTQVAxDDAKBgNVBAsMA1NBUDElMCMGCSqGSIb3DQEJARYWbWF5
YS5zaWdhbEBleGFtcGxlLmNvbTESMBAGA1UEAwwJbG9jYWxob3N0MIIBIjANBgkq
hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAzoozvGy0hXQGJmZ+Mmim3P+XSS35KRCr
KIoJwAWN+t6XeIFTmkYFk1hiDa165bbet79mu20eayWpku5xXYkVVciuU4KZr4M/
uO+cpgsbAzSiN53ddwwUjzgAs7WnYqA/0e9Dx1FSL0GVnU9HF7gvV6ym9qlDjB5f
aG4plvwZPXixw6qERfDHFSNBtXdydoJyJd1EhGFbUw/JWM5lKqf+tr4b2XgN+nst
/qBa8AQqV+hywkAyPvABAdRUHcErX6b5OUeKxbkkzOGqyWEAaPE/X/i9x5ROO5fP
NzNEwngEkBLsjwXPkdNBtkc7Ydn3eW6rdnuybzcqS3pf2NNQBoMq0QIDAQABo4HW
MIHTMIGcBgNVHSMEgZQwgZGhgYOkgYAwfjELMAkGA1UEBhMCVVMxCzAJBgNVBAgM
Ak5ZMQswCQYDVQQHDAJOWTEMMAoGA1UECgwDU0FQMQwwCgYDVQQLDANTQVAxJTAj
BgkqhkiG9w0BCQEWFm1heWEuc2lnYWxAZXhhbXBsZS5jb20xEjAQBgNVBAMMCWxv
Y2FsaG9zdIIJAM5G4IskOWh3MAkGA1UdEwQCMAAwCwYDVR0PBAQDAgTwMBoGA1Ud
EQQTMBGCCWxvY2FsaG9zdIcEfwAAATANBgkqhkiG9w0BAQsFAAOCAQEAb6+o3ciu
7Za33Vs827Zj5P3qTI97GeFDB/7UJARZWNzrsg4nel+dPUsUBQNxvHluiMfvMKMw
Hz2RUPP9HcwuNjdFC6ilcAhUYzP8ThP7oh/WrcjrdgRZlWOvlTsgU8r7wBngN2nd
cIlo7rcSFszP2dG95z9DBDWkTBbOZ33JUm9osjGFYa3Nwxh9vT7VBtJg9U0QSN53
Fee+9Sx5pYa72IbUf9c82uOW4yoYF6S+cP/1mQYDoh5iRoa5CvVIKY0Uieh6HvDs
pJ7VeSGbzGIBJvd2du5gFGT1xcuS0ieq9paKxhOE4bmX0PGt5qssKsL3rMTWRm4I
cHN4/LuuE/gO1g==
-----END CERTIFICATE-----`
					key = `-----BEGIN RSA PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDOijO8bLSFdAYm
Zn4yaKbc/5dJLfkpEKsoignABY363pd4gVOaRgWTWGINrXrltt63v2a7bR5rJamS
7nFdiRVVyK5Tgpmvgz+475ymCxsDNKI3nd13DBSPOACztadioD/R70PHUVIvQZWd
T0cXuC9XrKb2qUOMHl9obimW/Bk9eLHDqoRF8McVI0G1d3J2gnIl3USEYVtTD8lY
zmUqp/62vhvZeA36ey3+oFrwBCpX6HLCQDI+8AEB1FQdwStfpvk5R4rFuSTM4arJ
YQBo8T9f+L3HlE47l883M0TCeASQEuyPBc+R00G2Rzth2fd5bqt2e7JvNypLel/Y
01AGgyrRAgMBAAECggEAPzqB0ho5PW2igFj6IzZ0ds1sJAQF9fNbYoK3r2hD6dwA
5Ow6is0K4eu5wNQt/mr4Taozqgciu8yA2DFU1TylImjYLUqa/+cfN99qxk46C8Yu
LvaOGObC2IFdfaaLwp6qSvuDdV5I2ZyrT8g4TGOfYqjBSFvTCO83aAHpi4ZLt8xQ
1kdI5h1NiWMy7M0x0Y78IQWReH/luBhTkL+WfUAm3HaRyciI0kT5CUG8hd+tYcyZ
WVSRnfHV+bB1pFPwul5M0SkI0cAfP2YgBRUQDiSeMd3lxMkayLIbKLPN76shuJpq
+L5NlZchTMi+RbpIXliLyF/gXyvBTeODLldxDPeoHQKBgQD6u3XtDLH90cTnI8z9
glvW6bI//xpi+dky1cT3OPn+R6DQNHSWLosHgh6XpPXQ02DfrCXOkL5Sib5tsqMU
IgNQQCknh3JRe+uzPoy4ehPUZgfftjZZy09Tt5E1Nq9TJJdkFzU1fUxUIuYdrRA3
11kUEYfpO5o9tBSGaAyvTO3CFwKBgQDS4Q6J6PiCdoNkOmmq2rKPV8Izy3obzt7u
saV924vHSavyaSc2lXGOcASXi3cKZpurWzycojg3Lwe0+hVwXTTqUq6Ci7q8Tb2W
uyMFHr5uyQtMM6d7empzbI6/OtJ2S1a9PZNZwmVVNkH7PGes61OkA8psAN2Y+x46
ShFEh7WTVwKBgQC4Sw/L1FgD86riJjtnXuj4V7/QMEcJ1xGhvuTOvo9qKuX2A4hq
Vv2T4D1yQyr3elcrMNJ9OYDbFCnnYbVid/mtg+t8BZ+uawJ9No2ijwCCTxicg8cB
S2Ica8IMtgw6dZvdUv2mOlnfQeOYjntsQBpWmOgoM8oUbofjnxkrxMTBswKBgD+l
dYHiMr8NjfJ+Ps42W5Yv4olHbH9gHKDmNRCbZsCrV54+Zntu92sKHBixGyikd29s
hgqwW08sfqL8p+PV/daLRehYy+9xdzs7GAK/mLJPM324SWBXPjHAHgVRd5wEeRV8
tDBvH65sRdXSEWh7Ti8+haW7TSaTBDiLilKoswDZAoGBAOzvAvBt5Vl4PYu8Cw4g
14HjEsgm/SnKd+fZsyogb7C1iE/Z2q37kz+4LnszDu7rg598bAoZwb116Zg6NtIY
WAd64XYejR+b1HZ+bDfWBayBTTK2g5qPVUfrUF8UCqDlo5YG9On4O1QCnL/gERRA
TSTAhYWEQVZqRKYQMYGHpNlU
-----END RSA PRIVATE KEY-----`
				})

				It("should succeed", func() {
					var err error
					client, err = NewClient(context.TODO(), &ClientConfig{
						URL:            "http://google.com",
						ClientID:       "client",
						TokenURLSuffix: "oauth/token",
						TLSCertKey:     certificate,
						TLSPrivateKey:  key,
					}, nil)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should fail on bad certificate", func() {
					var err error
					client, err = NewClient(context.TODO(), &ClientConfig{
						URL:            "http://google.com",
						ClientID:       "client",
						TokenURLSuffix: "oauth/token",
						TLSCertKey:     "-----BEGIN CERTIFICATE-----aaa-----END CERTIFICATE-----",
						TLSPrivateKey:  key,
					}, nil)
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Describe("Unbind", func() {
			Context("when an existing binding is being deleted synchronously", func() {
				BeforeEach(func() {
					responseBody := []byte("{}")
					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceBindingsURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
					}
				})
				It("should be successfully removed", func() {
					location, err := client.Unbind(binding.ID, params, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(BeEmpty())
				})
			})

			Context("when an existing binding is being deleted asynchronously", func() {
				var locationHeader string
				BeforeEach(func() {
					locationHeader = "location"
					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceBindingsURL + "/", ResponseStatusCode: http.StatusAccepted, Headers: map[string]string{"Location": locationHeader}},
					}
				})
				It("should be successfully removed", func() {
					location, err := client.Unbind(binding.ID, params, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(Equal(locationHeader))
				})
			})

			Context("when service manager returns a non-expected status code", func() {
				BeforeEach(func() {
					responseBody := []byte("{}")

					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceBindingsURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusCreated},
					}
				})
				It("should handle error", func() {
					location, err := client.Unbind(binding.ID, params, "test-user")
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusCreated)
					Expect(location).Should(BeEmpty())
				})
			})

			Context("when service manager returns a status code not found", func() {
				BeforeEach(func() {
					responseBody := []byte(`{ "description": "Binding not found" }`)

					handlerDetails = []HandlerDetails{
						{Method: http.MethodDelete, Path: types.ServiceBindingsURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusNotFound},
					}
				})
				It("should be considered as success", func() {
					location, err := client.Unbind(binding.ID, params, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(location).Should(BeEmpty())
				})
			})
		})

		Describe("Rename Binding", func() {
			BeforeEach(func() {
				responseBody, _ := json.Marshal(binding)
				handlerDetails = []HandlerDetails{
					{Method: http.MethodPatch, Path: types.ServiceBindingsURL + "/" + binding.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
				}
			})

			It("should rename binding", func() {
				res, err := client.RenameBinding(binding.ID, "newname", "newk8sname")
				Expect(err).ToNot(HaveOccurred())
				Expect(res.ID).To(Equal("bindingID"))
			})
		})
	})

	It("build operation url", func() {
		opUrl := BuildOperationURL("5678", "1234", types.ServiceInstancesURL)
		Expect(opUrl).To(Equal("/v1/service_instances/1234/operations/5678"))
	})

	When("checking status of existing op", func() {
		var operation *types.Operation
		BeforeEach(func() {
			operation = &types.Operation{
				ID:         "operation-id",
				Type:       "create",
				State:      "failed",
				ResourceID: "1234",
			}
			responseBody, _ := json.Marshal(operation)
			handlerDetails = []HandlerDetails{
				{Method: http.MethodGet, Path: types.ServiceInstancesURL + "/", ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
			}
		})

		It("should return it", func() {
			result, err := client.Status(types.ServiceInstancesURL+"/1234/"+operation.ID, params)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(Equal(operation))
		})
	})
})

func expectErrorToContainSubstringAndStatusCode(err error, substring string, statusCode int) {
	Expect(err).Should(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(substring))
	Expect(err.(*ServiceManagerError).StatusCode).To(Equal(statusCode))
}
