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

		Describe("Share/Unshare", func() {
			BeforeEach(func() {
				responseBody, _ := json.Marshal(instance)
				handlerDetails = []HandlerDetails{
					{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusOK},
				}
			})
			When("When valid instance is being shared", func() {
				It("should be shared successfully", func() {
					err := client.ShareInstance(instance.ID, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
				})
			})

			When("When instance is being unshared", func() {
				It("should be unshared successfully", func() {
					err := client.UnShareInstance(instance.ID, "test-user")
					Expect(err).ShouldNot(HaveOccurred())
				})
			})

			When("When instance fails to be shared", func() {
				BeforeEach(func() {
					responseBody, _ := json.Marshal(instance)
					handlerDetails = []HandlerDetails{
						{Method: http.MethodPatch, Path: types.ServiceInstancesURL + "/" + instance.ID, ResponseBody: responseBody, ResponseStatusCode: http.StatusInternalServerError},
					}
				})
				It("returns error", func() {
					err := client.UnShareInstance(instance.ID, "test-user")
					Expect(err).Should(HaveOccurred())
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
					// fake certificate and key for testing only — CN=fake-localhost, not used in production
					certificate = `-----BEGIN CERTIFICATE-----
MIIDFTCCAf2gAwIBAgIUBfv6cKhSA0RFTKSrg2BpHjGbdacwDQYJKoZIhvcNAQEL
BQAwGTEXMBUGA1UEAwwOZmFrZS1sb2NhbGhvc3QwIBcNMjYxMTE5MTEzMTE2WhgP
MjA1NDA0MDUxMTMxMTZaMBkxFzAVBgNVBAMMDmZha2UtbG9jYWxob3N0MIIBIjAN
BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArvlpqA53uJOQWxbQEOQRnF9dxLqS
M7Za5eMx7/094w5/DByegUcNDGJYyxQL5epe2X4WnvYoFTW5rEuRsjtrgl8go1gN
Z43/GgTKRD+dZuEQXOTB2sk89SasUUaT2cSqVCdqCFXLiTS5FYTgU12gIvzELM1a
a1p6hHOhrcJX16zTPlPYovnxDeb1i98cCzV4Mk/l/qWEjSDf9OqBKLb1Ec6KGJYz
NgklInLSt1ScuQd4kMzkgkCWT185cMtLk9QXK5KGywHsFVRwsD1fyqUfkpLXR8xq
d/6yDmm5eXQUG+FufaFFStmjv9r3PhXa94jFZtVrmCi802ulA3LHmJIu6wIDAQAB
o1MwUTAdBgNVHQ4EFgQUxEiOvOo4QKO8Qb+pSEgpdZEbn5kwHwYDVR0jBBgwFoAU
xEiOvOo4QKO8Qb+pSEgpdZEbn5kwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0B
AQsFAAOCAQEAaeQBJnXHKF5n/XrVdEs5V3AyB0KqFoZSy1lq2kQsmxpeOKArDE44
bXz8/PhVsyGpKcgXaZPEpin5YJJCsr9hI7LHUUB5rHLiSBHtOGwvZylVG6wG3wci
M7lVvaQY9w8u91C7/aa751fKL7bYQZ6f9naNhUAZ9od0u/BL19svJT2FYrpqjyQU
DpoVe7goDDRsjFhsViTuU0Wq89Wc9YtTrpFKzaUx5G+jUsSCjtMcSBcCZU46ZHLD
ELJmTUMZ0++8InXmRnmYCNijmo7rS59F8PSFfqBZdK+B9rRa7kwQzIp99p1tBg57
4NYJa5fYkwa0AhpdOXt7BB7E3DXM26DpbA==
-----END CERTIFICATE-----`
					key = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCu+WmoDne4k5Bb
FtAQ5BGcX13EupIztlrl4zHv/T3jDn8MHJ6BRw0MYljLFAvl6l7Zfhae9igVNbms
S5GyO2uCXyCjWA1njf8aBMpEP51m4RBc5MHayTz1JqxRRpPZxKpUJ2oIVcuJNLkV
hOBTXaAi/MQszVprWnqEc6GtwlfXrNM+U9ii+fEN5vWL3xwLNXgyT+X+pYSNIN/0
6oEotvURzooYljM2CSUictK3VJy5B3iQzOSCQJZPXzlwy0uT1BcrkobLAewVVHCw
PV/KpR+SktdHzGp3/rIOabl5dBQb4W59oUVK2aO/2vc+Fdr3iMVm1WuYKLzTa6UD
cseYki7rAgMBAAECggEAF9xJLTKz3sa4s8M7omIHiMR0fARZNZYTdS/zOl/VVaY/
av5oGwb+X4b1xtO3/6VjQ9Nu8ERvGN1XsYgomR6DPacNQ1kZktqJRBoNHxtxsnrU
iWZkjRpOJrwcWYT/Wn34IDuzuOlCNTasWqPhF104Hi1L7zQdtXAELxW5oYFUC0P6
/TeVMhWkwMbaQhE6yXMP1eVmLj9G42Kz5U1hWnP6xZffmWz0YIK0un0WkOuT9ui2
c5cvzGcKkkG1OBNOeWBya7U1vTxf6MNBXjdKOrihToi8KJQIQMGPDQa0E+v0Yydv
IdYJdcQfjsyVjY/byN/LU3wPp3jGVdRTojhQh9Pt8QKBgQDtODD2/M1NM0Q9AtoU
gByN2Wt0nirti7B8i1XH2DrLfmIGadnZsHrOHo+eV1iUT1IlTKOgcXCJIk6dWOHM
37hQh+7XgM1quufSQngj8+RIC94cPxR4IHgdWm27OVmR1utWN+EHBjOLX7XAGqfy
XdNpiXTNJ/v29NvTZomNXmFFQwKBgQC8060U4Day929jWPB+xdo7aeaWzB0PaPk+
8n72rTpUcONirMz4LD0YkEgehMKwX33FuQsBcwMgTWWkm29HoNyBSQvEiBqFKLh3
i0jYNh3LQu5JgU7wZm0tpn/mdQnbd3etKUXkYsuVCmgtXcRvYfLh6kkC8CX2ARax
1HvW6UGBOQKBgQDsoSbG2bp5jfA95ABifsD9BqrX84bi2ns8YUk6Romvv+xMdX5o
VlDKF1YwHvUvTEinCGIYg9A8G0rMToM/eNNFEbPAqK1AEB17ce6h/oHzFFOFXa8v
fUQUE8ZuAnFEt1yd7rO77AWyu9BzRqPNcqaXGhS3KXGAERTPNfN0X3csPQKBgDaa
4AqRbEAE3elU5vdCMAHOdihVjnHOYYWaiqy7G4sfuiHm5r89y802rCRnMAXhF3X+
NPiOUfEdcK9RY7TLB/k7LnP/B+xKAk5PsRkVTThiepY3FqTeeeQhqgry/Y7B1ZH1
xYhuKYFIKSg1XYD4Skyae9Klr9ScK/vw/n7/pTnBAoGAFxQZMvfqWT7LarLm8Cf1
0pZoMUfgMKWl2shvl1HnPR3gOdAlIDfCE2QydcG+C64+SzZtd8+9auZA7XL/lf5N
RTISzOGZR6Dcpe8US4/P6U3A6oE350xNc3gKIjqumUQ57YkaGtPZbrM8kfcq15J2
qs/jqUJUAtEF6j0Gw6ZirKw=
-----END PRIVATE KEY-----`
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

	Context("Status", func() {
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

		It("should return the operation when exist", func() {
			result, err := client.Status(types.ServiceInstancesURL+"/1234/"+operation.ID, types.CREATE, params)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(Equal(operation))
		})

		When("SM returned 404", func() {
			When("last operation is delete", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL + "/", ResponseStatusCode: http.StatusNotFound},
					}
				})
				It("should return succeeded operation", func() {
					result, err := client.Status(types.ServiceInstancesURL+"/1234/"+operation.ID, types.DELETE, params)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(result.State).To(Equal(types.SUCCEEDED))
				})
			})

			When("last operation is not delete", func() {
				BeforeEach(func() {
					handlerDetails = []HandlerDetails{
						{Method: http.MethodGet, Path: types.ServiceInstancesURL + "/", ResponseStatusCode: http.StatusNotFound},
					}
				})
				It("should fail for create", func() {
					_, err := client.Status(types.ServiceInstancesURL+"/1234/"+operation.ID, types.CREATE, params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusNotFound)
				})
				It("should fail for update", func() {
					_, err := client.Status(types.ServiceInstancesURL+"/1234/"+operation.ID, types.UPDATE, params)
					expectErrorToContainSubstringAndStatusCode(err, "", http.StatusNotFound)
				})
			})
		})

	})
})

func expectErrorToContainSubstringAndStatusCode(err error, substring string, statusCode int) {
	Expect(err).Should(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(substring))
	Expect(err.(*ServiceManagerError).StatusCode).To(Equal(statusCode))
}
