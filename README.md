[![Coverage Status](https://coveralls.io/repos/github/sm-operator/sapcp-operator/badge.svg?branch=master&killcache=1)](https://coveralls.io/github/sm-operator/sapcp-operator?branch=master)
[![Build Status](https://github.com/sm-operator/sapcp-operator/workflows/Go/badge.svg)](https://github.com/sm-operator/sapcp-operator/actions)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/sm-operator/sapcp-operator/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/sm-operator/sapcp-operator)](https://goreportcard.com/report/github.com/sm-operator/sapcp-operator)

# SAP Business Technology Platform Service Operator


With the SAP Business Technology Platform (SAP BTP) Operator, you can provision and bind SAP BTP services to your Kubernetes cluster in a Kubernetes-native way. The SAP BTP Service Operator is based on the [Kubernetes custom resource definition (CRD) API](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) so that your applications can create, update, and delete SAP CP services from within the cluster by calling Kubnernetes APIs.

## Table of content
* [Prerequisites](#prerequisites)
* [Setup Operator](#setup)
* [Local Setup](#local-setup)
* [SAP BTP kubectl extension](#sapbtp-kubectl-extension-experimental)
* [Using the SAP BTP Service Operator](#using-the-sapbtp-operator)
    * [Creating a service instance](#step-1-creating-a-service-instance)
    * [Binding the service instance](#step-2-binding-the-service-instance)
* [Reference documentation](#reference-documentation)
    * [Service instance properties](#service-instance-properties)
    * [Binding properties](#binding-properties)    

## Prerequisites
- SAP Cloud Platform [Global Account](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/d61c2819034b48e68145c45c36acba6e.html) and [Subaccount](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/55d0b6d8b96846b8ae93b85194df0944.html) 
- [Kubernetes cluster](https://kubernetes.io/) running version 1.17 or higher 
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) v1.17 or higher
- [helm](https://helm.sh/) v3.0 or higher

[Back to top](#sap-business-technology-platform-service-operator)

## Setup
1. Install [cert-manager](https://cert-manager.io/docs/installation/kubernetes)

1. Obtain access credentials for the SAP BTP Service Operator:
   1. Using SAP BTP Cockpit or CLI, create an instance of the Service Management (`service-manager`) service, plan `service-operator-access`
      
      More information about creating service instances is available here: 
      [Cockpit](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/bf71f6a7b7754dbd9dfc2569791ccc96.html), 
      [CLI](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/b327b66b711746b085ec5d2ea16e608e.html)  
   
   1. Create a binding to the created service instance
      
      More information about creating service bindings is available here: 
            [Cockpit](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/bf71f6a7b7754dbd9dfc2569791ccc96.html), 
            [CLI](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/f53ff2634e0a46d6bfc72ec075418dcd.html) 
   
   1. Retrieve the generated access credentials from the created binding.
   
       The generated access credentials will available in the created binding, for example:
       ```json
        {
            "clientid": "xxxxxxx",
            "clientsecret": "xxxxxxx",
            "url": "https://mysubaccount.authentication.eu10.hana.ondemand.com",
            "xsappname": "b15166|service-manager!b1234",
            "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
        }
       ```  
   
1. Deploy the sapbtp-service-operator in the cluster using the obtained access credentials:
    ```bash
    helm upgrade --install sapcp-operator https://github.com/sm-operator/sapcp-operator/releases/download/<release>/sapcp-operator-<release>.tgz \
        --create-namespace \
        --namespace=sapbtp-operator \
        --set manager.secret.clientid=<clientid> \
        --set manager.secret.clientsecret=<clientsecret> \
        --set manager.secret.url=<sm_url> \
        --set manager.secret.tokenurl=<url>
    ```

    The list of available releases is available here: [sapbtp-operator releases](https://github.com/sm-operator/sapcp-operator/releases)

[Back to top](#sap-business-technology-platform-service-operator)

## Using the SAP BTP Service Operator

#### Step 1: Create a service instance

1.  To create an instance of a SAP BTP service, first create a `ServiceInstance` custom resource file:

```yaml
    apiVersion: services.cloud.sap.com/v1alpha1
    kind: ServiceInstance
    metadata:
        name: my-service-instance
    spec:
        serviceOfferingName: <offering>
        servicePlanName: <plan>
   ```

   *   `<offering>` is the name of the SAP BTP service that you want to create. 
       You can find the list of available services in the SAP BTP Cockpit, see [Service Marketplace](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/55b31ea23c474f6ba2f64ee4848ab1b3.html).
   *   `<plan>` is the plan of the selected service offering that you want to create.

2.  Apply the custom resource file in your cluster to create the instance.

    ```bash
    kubectl apply -f path/to/my-service-instance.yaml
    ```

3.  Check that your service status is **Created** in your cluster.
    
    //TODO update example output with all fields
    
    ```bash
    kubectl get serviceinstances
    NAME                  STATUS   AGE
    my-service-instance   Created  19s
    ```
[Back to top](#sap-business-technology-platform-service-operator)

#### Step 2: Create a Service Binding

1.  To get access credentials to your service instance and make them available in the cluster so that your applications can use it, create a `ServiceBinding` custom resource, and set the `serviceInstanceName` field to the name of the `ServiceInstance` resource you created.

    ```yaml
    apiVersion: services.cloud.sap.com/v1alpha1
    kind: ServiceBinding
    metadata:
        name: my-binding
    spec:
        serviceInstanceName: my-service-instance
    ```

1.  Apply the custom resource file in your cluster to create the binding.

    ```bash
    kubectl apply -f path/to/my-binding.yaml
    ```

1.  Check that your binding status is **Created**.

    ```bash
    kubectl get servicebindings
    NAME         INSTANCE              STATUS    AGE
    my-binding   my-service-instance   Created   16s
    
    ```

1.  Check that a secret of the same name as your binding is created. The secret contains the service credentials that apps in your cluster can use to access the service.

    ```bash
    kubectl get secrets
    NAME         TYPE     DATA   AGE
    my-binding   Opaque   5      32s
    ```
    
    See [Using Secrets](https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets) for the different options to use the credentials from your application running in the Kubernetes cluster, 

[Back to top](#sap-business-technology-platform-service-operator)

## Reference documentation

### Service Instance
#### Spec
| Property         | Type     | Comments                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| serviceOfferingName`*`   | `string`   | The SAP BTP service offering name |
| servicePlanName`*` | `string`   |  The plan to use for the service instance |
| servicePlanID   |  `string`   |  The plan ID in case service offering and plan name are ambiguous |
| externalName       | `string`   |  The name for the service instance in SAP BTP, defaults to the binding `metadata.name` if not specified |
| parameters       |  `[]object`  |  Provisioning parameters for the instance, check the documentation of the specific service you are using for details |

#### Status
| Property         | Type     | Comments                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| instanceID   | `string`   | The service instance ID in SAP BTP Service Management |
| operationURL | `string`   |  URL of ongoing operation for the service instance |
| operationType   |  `string`   |  The operation type (CREATE/UPDATE/DELETE) for ongoing operation |
| conditions       | `[]condition`   |  An array of conditions describing the status of the service instance. <br>The possible conditions types are:<br>- `Ready`: set to `true` if the instance is ready and usable<br>- `Failed`: set to `true` when an operation on the service instance fails, in this case the error details will be available in the condition message  



### Service Binding 
#### Spec
| Parameter             | Type       | Comments                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| serviceInstanceName`*`   | `string`   | The Kubernetes name of the service instance to bind, should be in the namespace of the binding |
| externalName       | `string`   |  The name for the service binding in SAP BTP Service Management, defaults to the binding `metadata.name` if not specified |
| secretName       | `string`   |  The name of the secret where credentials will be stored, defaults to the binding `metadata.name` if not specified |
| parameters       |  `[]object`  |  Parameters for the binding |

#### Status
| Property         | Type     | Comments                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| instanceID   | `string`   | The ID of the bound instance in SAP BTP Service Management |
| bindingID   | `string`   | The service binding ID in SAP BTP Service Management |
| operationURL | `string`   |  URL of ongoing operation for the service binding |
| operationType   |  `string`   |  The operation type (CREATE/UPDATE/DELETE) for ongoing operation |
| conditions       | `[]condition`   |  An array of conditions describing the status of the service instance. <br>The possible conditions types are:<br>- `Ready`: set to `true` if the binding is ready and usable<br>- `Failed`: set to `true` when an operation on the service binding fails, in this case the error details will be available in the condition message  

[Back to top](#sap-business-technology-platform-service-operator)

## Support
Feel free to open new issues for feature requests, bugs or general feedback on the GitHub issues page of this project. 
The SAP BTP Service Operator project maintainers will respond to the best of their abilities. 

## Contributions
We currently do not accept community contributions. 

## SAP BTP kubectl Plugin (Experimental) 
The SAP BTP kubectl plugin extends kubectl with commands for getting the available services in your SAP BTP account, 
using the access credentials stored in the cluster.

### Prerequisites
- [jq](https://stedolan.github.io/jq/)

### Limitations
- The SAP BTP kubectl plugin is currently based on `bash`. If using Windows, you should use the SAP BTP plugin commands from a linux shell (e.g. [Cygwin](https://www.cygwin.com/)).  

### Installation
- Download https://github.com/sm-operator/sapcp-operator/releases/download/${release}/kubectl-sapcp
- Move the executable file to anywhere on your `PATH`

#### Usage
```
  kubectl sapbtp marketplace -n <namespace>
  kubectl sapbtp plans -n <namespace>
  kubectl sapbtp services -n <namespace>
```

Use the `namespace` parameter to specify the location of the secret containing the SAP BTP access credentials, usually the namespace in which you installed the operator. 
If not specified the `default` namespace is used. 


[Back to top](#sap-business-technology-platform-service-operator)
