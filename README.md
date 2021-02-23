[![Coverage Status](https://coveralls.io/repos/github/SAP/sap-btp-service-operator/badge.svg?branch=main)](https://coveralls.io/github/SAP/sap-btp-service-operator?branch=main)
[![Build Status](https://github.com/SAP/sap-btp-service-operator/workflows/Go/badge.svg)](https://github.com/SAP/sap-btp-service-operator/actions)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/SAP/sap-btp-service-operator/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/SAP/sap-btp-service-operator)](https://goreportcard.com/report/github.com/SAP/sap-btp-service-operator)
[![REUSE status](https://api.reuse.software/badge/github.com/SAP/sap-btp-service-operator)](https://api.reuse.software/info/github.com/SAP/sap-btp-service-operator)

# SAP Business Technology Platform (SAP BTP) Service Operator for Kubernetes

With the SAP BTP service operator, you can consume [SAP BTP services](https://platformx-d8bd51250.dispatcher.us2.hana.ondemand.com/protected/index.html#/viewServices?) from your Kubernetes cluster using Kubernetes-native tools. 
SAP BTP service operator allows you to provision and manage service instances and service bindings of SAP BTP services so that your Kubernetes-native applications can access and use needed services from the cluster.  
The SAP BTP service operator is based on the [CustomResourceDefinitions API](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/).

## Note
This feature is still under development, review, and testing. 

## Table of content
* [Prerequisites](#prerequisites)
* [Setup Operator](#setup)
* [SAP BTP kubectl extension](#sap-btp-kubectl-plugin-experimental)
* [Using the SAP BTP Service Operator](#using-the-sap-btp-service-operator)
    * [Creating a service instance](#step-1-create-a-service-instance)
    * [Binding the service instance](#step-2-create-a-service-binding)
* [Reference documentation](#reference-documentation)
    * [Service instance properties](#service-instance)
    * [Binding properties](#service-binding)    

## Prerequisites
- SAP BTP [Global Account](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/d61c2819034b48e68145c45c36acba6e.html) and [Subaccount](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/55d0b6d8b96846b8ae93b85194df0944.html) 
- Service Management Control (SMCTL) Command Line Interface. See [Using the SMCTL](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/0107f3f8c1954a4e96802f556fc807e3.html).
- [Kubernetes cluster](https://kubernetes.io/) running version 1.17 or higher 
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) v1.17 or higher
- [helm](https://helm.sh/) v3.0 or higher

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Setup
1. Install [cert-manager](https://cert-manager.io/docs/installation/kubernetes)

2. Obtain the access credentials for the SAP BTP service operator:

   a. Using the SAP BTP Cockpit or CLI, create an instance of the SAP Cloud Service Management service (technical name: `service-manager`) with the plan:
    `service-operator-access`
      
      For more information about creating service instances, see:     
      * [Creating Service Instances Using the SAP BTP Cockpit](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/bf71f6a7b7754dbd9dfc2569791ccc96.html)
        
      * [Creating Service Instances using SMCTL](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/b327b66b711746b085ec5d2ea16e608e.html)  
   
   b. Create a binding to the created service instance.
      
   For more information about creating service bindings, see:  
      * [Creating Service Bindings Using the SAP BTP Cockpit](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/55b31ea23c474f6ba2f64ee4848ab1b3.html) 
       
      * [Creating Service Bindings Using SMCTL](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/f53ff2634e0a46d6bfc72ec075418dcd.html). 
   
   c. Retrieve the generated access credentials from the created binding:
   
      The example of the credentials in the binding object
      
       ```json
        {
            "clientid": "xxxxxxx",
            "clientsecret": "xxxxxxx",
            "url": "https://mysubaccount.authentication.eu10.hana.ondemand.com",
            "xsappname": "b15166|service-manager!b1234",
            "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
        }
       ```  
   
3. Deploy the the SAP BTP service operator in the cluster using the obtained access credentials:
    ```bash
    helm upgrade --install sapbtp-operator https://github.com/SAP/sap-btp-service-operator/releases/download/<release>/sapbtp-operator-<release>.tgz \
        --create-namespace \
        --namespace=sapbtp-operator \
        --set manager.secret.clientid=<clientid> \
        --set manager.secret.clientsecret=<clientsecret> \
        --set manager.secret.url=<sm_url> \
        --set manager.secret.tokenurl=<url>
    ```

    The list of available releases: [sapbtp-operator releases](https://github.com/SAP/sap-btp-service-operator/releases)

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes).

## Using the SAP BTP Service Operator

#### Step 1: Create a service instance

1.  To create an instance of a service offered by SAP BTP, first create a `ServiceInstance` custom-resource file:

```yaml
    apiVersion: services.cloud.sap.com/v1alpha1
    kind: ServiceInstance
    metadata:
        name: my-service-instance
    spec:
        serviceOfferingName: <offering>
        servicePlanName: <plan>
   ```

   *   `<offering>` - The name of the SAP BTP service that you want to create. 
       To learn more about viewing and managing the available services for your subaccount in the SAP BTP cockpit, see [Service Marketplace](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/affcc245c332433ba71917ff715b9971.html). 
        
        Tip: Use the *Environment* filter to get all offerings that are relevant for Kubernetes.
   *   `<plan>` - The plan of the selected service offering that you want to create.

2.  Apply the custom-resource file in your cluster to create the instance.

    ```bash
    kubectl apply -f path/to/my-service-instance.yaml
    ```

3.  Check that the status of the service in your cluster is **Created**.
    
    //TODO update example output with all fields
    
    ```bash
    kubectl get serviceinstances
    NAME                  STATUS   AGE
    my-service-instance   Created  19s
    ```
[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

#### Step 2: Create a Service Binding

1.  To get access credentials to your service instance and make it available in the cluster so that your applications can use it, create a `ServiceBinding` custom resource, and set the `serviceInstanceName` field to the name of the `ServiceInstance` resource you created.

    ```yaml
    apiVersion: services.cloud.sap.com/v1alpha1
    kind: ServiceBinding
    metadata:
        name: my-binding
    spec:
        serviceInstanceName: my-service-instance
    ```

2.  Apply the custom resource file in your cluster to create the binding.

    ```bash
    kubectl apply -f path/to/my-binding.yaml
    ```

3.  Check that your binding status is **Created**.

    ```bash
    kubectl get servicebindings
    NAME         INSTANCE              STATUS    AGE
    my-binding   my-service-instance   Created   16s
    
    ```

4.  Check that a secret with the same name as the name of your binding is created. The secret contains the service credentials that apps in your cluster can use to access the service.

    ```bash
    kubectl get secrets
    NAME         TYPE     DATA   AGE
    my-binding   Opaque   5      32s
    ```
    
    See [Using Secrets](https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets) to learn about different options on how to use the credentials from your application running in the Kubernetes cluster, 

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Reference documentation

### Service Instance
#### Spec
| Property         | Type     | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| serviceOfferingName`*` | `string`  | The name of the SAP BTP service offering. |
| servicePlanName`*` | `string`   |  The plan to use for the service instance. |
| servicePlanID   |  `string`   |  The plan ID in case service offering and plan name are ambiguous. |
| externalName       | `string`   |  The name for the service instance in SAP BTP, defaults to the instance `metadata.name` if not specified. |
| parameters       |  `[]object`  |  Some services support the provisioning of additional configuration parameters during the instance creation. For the list of supported                                  parameters, check the documentation of the particular service offering.|

#### Status
| Property         | Type     | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| instanceID   | `string`   | The service instance ID in SAP Cloud Service Management service. |
| operationURL | `string`   |  The URL of the current operation performed on the service instance. |
| operationType   |  `string`   |  The type of the current operation. Valid values are CREATE, UPDATE, or DELETE.  |
| conditions       | `[]condition`   |  An array of conditions describing the status of the service instance. <br>The possible condition types are:<br>- `Ready`: set to `true`  if the instance is ready and usable<br>- `Failed`: set to `true` when an operation on the service instance fails. In this case, the details about the error are available in the        condition message.



### Service Binding 
#### Spec
| Parameter             | Type       | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| serviceInstanceName`*`   | `string`   | The Kubernetes name of the service instance to bind, should be in the namespace of the binding. |
| externalName       | `string`   |  The name for the service binding,, defaults to the binding `metadata.name` if not specified |
| secretName       | `string`   |  The name of the secret where the credentials are stored, defaults to the binding `metadata.name` if not specified |
| parameters       |  `[]object`  |  Some services support the provisioning of additional configuration parameters during the bind request. For the list of supported                                  parameters, check the documentation of the particular service offering. |

#### Status
| Property         | Type     | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| instanceID   | `string`   | The ID of the bound instance in the SAP Cloud Service Management service. |
| bindingID   | `string`   | The service binding ID in SAP Cloud Service Management service.|
| operationURL | `string`   |  The URL of the current operation performed on the service binding. |
| operationType   |  `string`   |  The type of the current operation. Valid values are CREATE, UPDATE, or DELETE. |
| conditions       | `[]condition`   |  An array of conditions describing the status of the service instance. <br>The possible conditions types are:<br>- `Ready`: set to `true` if the binding is ready and usable<br>- `Failed`: set to `true` when an operation on the service binding fails. In this case, the details about the error are available in the     condition message.  

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Support
You're welcome to raise issues related to feature requests, bugs, or give us general feedback on this project's GitHub Issues page. 
The SAP BTP service operator project maintainers will respond to the best of their abilities. 

## Contributions
We currently do not accept community contributions. 

## SAP BTP kubectl Plugin (Experimental) 
The SAP BTP kubectl plugin extends kubectl with commands for getting the available services in your SAP BTP account by
using the access credentials stored in the cluster.

### Prerequisites
- [jq](https://stedolan.github.io/jq/)

### Limitations
- The SAP BTP kubectl plugin is currently based on `bash`. If you're using Windows, you should utilize the SAP BTP plugin commands from a linux shell (e.g. [Cygwin](https://www.cygwin.com/)).  

### Installation
- Download https://github.com/SAP/sap-btp-service-operator/releases/download/${release}/kubectl-sapbtp
- Move the executable file to any location in your `PATH`

### Usage
```
  kubectl sapbtp marketplace -n <namespace>
  kubectl sapbtp plans -n <namespace>
  kubectl sapbtp services -n <namespace>
```

Use the `namespace` parameter to specify the location of the secret containing the SAP BTP access credentials.  
Usually it is the namespace in which you installed the operator. 
If not specified, the `default` namespace is used. 


[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Licence
This project is licensed under Apache 2.0 except as noted otherwise in the [license](./LICENSE) file.
