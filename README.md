[![Coverage Status](https://coveralls.io/repos/github/SAP/sap-btp-service-operator/badge.svg?branch=main)](https://coveralls.io/github/SAP/sap-btp-service-operator?branch=main)
[![Build Status](https://github.com/SAP/sap-btp-service-operator/workflows/Go/badge.svg)](https://github.com/SAP/sap-btp-service-operator/actions)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/SAP/sap-btp-service-operator/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/SAP/sap-btp-service-operator)](https://goreportcard.com/report/github.com/SAP/sap-btp-service-operator)
[![REUSE status](https://api.reuse.software/badge/github.com/SAP/sap-btp-service-operator)](https://api.reuse.software/info/github.com/SAP/sap-btp-service-operator)

# SAP Business Technology Platform (SAP BTP) Service Operator for Kubernetes

With the SAP BTP service operator, you can consume [SAP BTP services](https://platformx-d8bd51250.dispatcher.us2.hana.ondemand.com/protected/index.html#/viewServices?) from your Kubernetes cluster using Kubernetes-native tools. 
SAP BTP service operator allows you to provision and manage service instances and service bindings of SAP BTP services so that your Kubernetes-native applications can access and use needed services from the cluster.  
The SAP BTP service operator is based on the [Kubernetes Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

## Table of Contents
* [Architecture](#architecture)
* [Prerequisites](#prerequisites)
* [Setup](#setup)
* [Versions](#versions)
* [Using the SAP BTP Service Operator](#using-the-sap-btp-service-operator)
    * [Creating a service instance](#step-1-create-a-service-instance)
    * [Binding the service instance](#step-2-create-a-service-binding)
* [Reference Documentation](#reference-documentation)
    * [Service instance properties](#service-instance)
    * [Binding properties](#service-binding)
    * [Passing parameters](#passing-parameters)
    * [Managing access](#managing-access)
* [SAP BTP kubectl Extension](#sap-btp-kubectl-plugin-experimental) 
* [Credentials Rotation](#credentials-rotation)
* [Multitenancy](#multitenancy)
* [Troubleshooting and Support](#troubleshooting-and-support)
* [Uninstalling the Operator](#uninstalling-the-operator)

## Architecture
SAP BTP service operator communicates with [Service Manager](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/3a27b85a47fc4dff99184dd5bf181e14.html) that uses the [Open service broker API](https://github.com/openservicebrokerapi/servicebroker) to communicate with service brokers, acting as an intermediary for the Kubernetes API Server to negotiate the initial provisioning and retrieve the credentials necessary for the application to use a managed service.<br><br>
It is implemented using a [CRDs-based](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#custom-resources) architecture.

![img](./docs/images/architecture.png)

## Prerequisites
- SAP BTP [Global Account](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/d61c2819034b48e68145c45c36acba6e.html) and [Subaccount](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/55d0b6d8b96846b8ae93b85194df0944.html) 
- Service Management Control (SMCTL) command line interface. See [Using the SMCTL](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/0107f3f8c1954a4e96802f556fc807e3.html).
- [Kubernetes cluster](https://kubernetes.io/) running version 1.17 or higher 
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) v1.17 or higher
- [helm](https://helm.sh/) v3.0 or higher

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Setup
1. Install [cert-manager](https://cert-manager.io/docs/installation/kubernetes)
   - for releases v0.1.18 or higher use cert manager v1.6.0 or higher 
   - for releases v0.1.17 or lower use cert manager lower then v1.6.0

2. Obtain the access credentials for the SAP BTP service operator:

   a. Using the SAP BTP cockpit or CLI, create an instance of the SAP Service Manager service (technical name: `service-manager`) with the plan:
    `service-operator-access`<br/><br>*Note*<br/><br>*If you can't see the needed plan, you need to entitle your subaccount to use SAP Service Manager service.*<br>

      *For more information about how to entitle a service to a subaccount, see:*
      * *[Configure Entitlements and Quotas for Subaccounts](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/5ba357b4fa1e4de4b9fcc4ae771609da.html)*
      
      
      <br/>For more information about creating service instances, see:     
      * [Creating Service Instances Using the SAP BTP Cockpit](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/bf71f6a7b7754dbd9dfc2569791ccc96.html)
        
      * [Creating Service Instances using SMCTL](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/b327b66b711746b085ec5d2ea16e608e.html)<br> 
   
   b. Create a binding to the created service instance.
      
   For more information about creating service bindings, see:  
      * [Creating Service Bindings Using the SAP BTP Cockpit](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/55b31ea23c474f6ba2f64ee4848ab1b3.html) 
       
      * [Creating Service Bindings Using SMCTL](https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/f53ff2634e0a46d6bfc72ec075418dcd.html). 
   
   c. Retrieve the generated access credentials from the created binding:
   
      The example of the default binding object used if no credentials type is specified:
      
    ```json
     {
         "clientid": "xxxxxxx",
         "clientsecret": "xxxxxxx",
         "url": "https://mysubaccount.authentication.eu10.hana.ondemand.com",
         "xsappname": "b15166|service-manager!b1234",
         "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
     }
    ```
    The example of the binding object with the specified X.509 credentials type:
    
    ```json
    {
         "clientid": "xxxxxxx",
         "certificate": "-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----..-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n",
         "key": "-----BEGIN RSA PRIVATE KEY-----...-----END RSA PRIVATE KEY-----\n",
         "certurl": "https://mysubaccount.authentication.cert.eu10.hana.ondemand.com",
         "xsappname": "b15166|service-manager!b1234",
         "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
     }
    ```
3. Add SAP BTP service operator chart repository  
   ```bash
    helm repo add sap-btp-operator https://sap.github.io/sap-btp-service-operator
   ```
4. Deploy the the SAP BTP service operator in the cluster using the obtained access credentials:<br>
   
   *Note:<br>
    If you are deploying the SAP BTP service operator in the registered cluster based on the Service Catalog (svcat) and Service Manager agent so that you can migrate svcat-based content to service operator-based content, add ```--set cluster.id=<clusterID>  ``` to your deployment script.*<br>*For more information, see the step 2 of the Setup section of [Migration to SAP BTP service operator](https://github.com/SAP/sap-btp-service-operator-migration/blob/main/README.md).*
   
   The example of the deployment that uses the default access credentials type:
    ```bash
    helm upgrade --install <release-name> sap-btp-operator/sap-btp-operator \
        --create-namespace \
        --namespace=sap-btp-operator \
        --set manager.secret.clientid=<clientid> \
        --set manager.secret.clientsecret=<clientsecret> \
        --set manager.secret.sm_url=<sm_url> \
        --set manager.secret.tokenurl=<url>
    ```
   The example of the deployment that uses the X.509 access credentials type:
    ```bash
    helm upgrade --install <release-name> sap-btp-operator/sap-btp-operator \
        --create-namespace \
        --namespace=sap-btp-operator \
        --set manager.secret.clientid=<clientid> \
        --set manager.secret.tls.crt="$(cat /path/to/cert)" \
        --set manager.secret.tls.key="$(cat /path/to/key)" \
        --set manager.secret.sm_url=<sm_url> \
        --set manager.secret.tokenurl=<certurl>
    ```
**Note:**<br> In order to rotate the credentials between the BTP service operator and Service Manager, you have to create a new binding for the service-operator-access service instance, and then to execute the setup script again, with the new set of credentials. Afterwards you can delete the old binding.
        

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes).

## Versions
Review the supported Kubernetes API versions for the following SAP BTP Service Operator versions.

| Operator version | Kubernetes API version |
| --- | --- |
| `v0.2` or later | `v1` |
| `v0.1` | `v1alpha1` |

## Using the SAP BTP Service Operator

#### Step 1: Create a service instance

1.  To create an instance of a service offered by SAP BTP, first create a `ServiceInstance` custom-resource file:

```yaml
    apiVersion: services.cloud.sap.com/v1
    kind: ServiceInstance
    metadata:
        name: my-service-instance
    spec:
        serviceOfferingName: sample-service
        servicePlanName: sample-plan
        externalName: my-service-instance-external
        parameters:
          key1: val1
          key2: val2
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

    ```bash
    kubectl get serviceinstances
    NAME                  OFFERING          PLAN        STATUS    AGE
    my-service-instance   <offering>        <plan>      Created   44s
    ```
[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

#### Step 2: Create a Service Binding

1.  To get access credentials to your service instance and make it available in the cluster so that your applications can use it, create a `ServiceBinding` custom resource, and set the `serviceInstanceName` field to the name of the `ServiceInstance` resource you created.

    The credentials are stored in a secret created in your cluster.

  ```yaml
    apiVersion: services.cloud.sap.com/v1
    kind: ServiceBinding
    metadata:
        name: my-binding
    spec:
        serviceInstanceName: my-service-instance
        externalName: my-binding-external
        secretName: mySecret
        parameters:
          key1: val1
          key2: val2      
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

## Reference Documentation

### Service Instance
#### Spec
| Parameter         | Type     | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| serviceOfferingName`*` | `string` | The name of the SAP BTP service offering. |
| servicePlanName`*` | `string` |  The plan to use for the service instance.   |
| servicePlanID   |  `string`  | The plan ID in case service offering and plan name are ambiguous. |
| externalName       | `string` | The name for the service instance in SAP BTP, defaults to the instance `metadata.name` if not specified. |
| parameters       | `[]object` | Some services support the provisioning of additional configuration parameters during the instance creation.<br/>For the list of supported parameters, check the documentation of the particular service offering. |
| parametersFrom | `[]object` | List of sources to populate parameters. |
| customTags | `[]string` | List of custom tags describing the ServiceInstance, will be copied to `ServiceBinding` secret in the key called `tags`. |
| userInfo | `object` | Contains information about the user that last modified this service instance. | 

#### Status
| Parameter         | Type     | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| instanceID   | `string` | The service instance ID in SAP Service Manager service.  |
| operationURL | `string` | The URL of the current operation performed on the service instance.  |
| operationType   |  `string`| The type of the current operation. Possible values are CREATE, UPDATE, or DELETE. |
| conditions       |  `[]condition`   | An array of conditions describing the status of the service instance.<br/>The possible condition types are:<br>- `Ready`: set to `true`  if the instance is ready and usable<br/>- `Failed`: set to `true` when an operation on the service instance fails.<br/> In the case of failure, the details about the error are available in the condition message.<br>- `Succeeded`: set to `true` when an operation on the service instance succeeded. In case of `false` operation considered as in progress unless `Failed` condition exists.
| tags       |  `[]string`   | Tags describing the ServiceInstance as provided in service catalog, will be copied to `ServiceBinding` secret in the key called `tags`.



### Service Binding 
#### Spec
| Parameter             | Type       | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| serviceInstanceName`*`   | `string`   |  The Kubernetes name of the service instance to bind, should be in the namespace of the binding. |
| externalName       | `string`   |  The name for the service binding in SAP BTP, defaults to the binding `metadata.name` if not specified. |
| secretName       | `string`   |  The name of the secret where the credentials are stored, defaults to the binding `metadata.name` if not specified. |
| secretKey | `string`  | The key inside the binding secret to store the credentials returned by the broker encoded as json to support complex data structures. |
| secretRootKey | `string`  | The key inside the secret to store all binding data including credentials returned by the broker and additional info under single key.<br/>Convenient way to store whole binding data in single file when using `volumeMounts`. |
| parameters       |  `[]object`  |  Some services support the provisioning of additional configuration parameters during the bind request.<br/>For the list of supported parameters, check the documentation of the particular service offering.|
| parametersFrom | `[]object` | List of sources to populate parameters. |
| userInfo | `object`  | Contains information about the user that last modified this service binding. |
| credentialsRotationPolicy | `object`  | Holds automatic credentials rotation configuration. |
| credentialsRotationPolicy.enabled | `boolean`  | Indicates whether automatic credentials rotation are enabled. |
| credentialsRotationPolicy.rotationFrequency | `duration`  | Specifies the frequency at which the binding rotation is performed. |
| credentialsRotationPolicy.rotatedBindingTTL | `duration`  | Specifies the time period for which to keep the rotated binding. |



#### Status
| Parameter         | Type     | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| instanceID   |  `string`  | The ID of the bound instance in the SAP Service Manager service. |
| bindingID   |  `string`  | The service binding ID in SAP Service Manager service. |
| operationURL |`string`| The URL of the current operation performed on the service binding. |
| operationType| `string `| The type of the current operation. Possible values are CREATE, UPDATE, or DELETE. |
| conditions| `[]condition` | An array of conditions describing the status of the service instance.<br/>The possible conditions types are:<br/>- `Ready`: set to `true` if the binding is ready and usable<br/>- `Failed`: set to `true` when an operation on the service binding fails.<br/> In the case of failure, the details about the error are available in the condition message.<br>- `Succeeded`: set to `true` when an operation on the service binding succeeded. In case of `false` operation considered as in progress unless `Failed` condition exists.
| lastCredentialsRotationTime| `time` | Indicates the last time the binding secret was rotated.

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

### Passing Parameters
To set input parameters, you may use the `parameters` and `parametersFrom`
fields in the `spec` field of the `ServiceInstance` or `ServiceBinding` resource:
- `parameters` : can be used to specify a set of properties to be sent to the
  broker. The data specified will be passed "as-is" to the broker without any
  modifications - aside from converting it to JSON for transmission to the broker
  in the case of the `spec` field being specified as `YAML`. Any valid `YAML` or
  `JSON` constructs are supported. Only one parameters field may be specified per
  `spec`.
- `parametersFrom` : can be used to specify which secret, and key in that secret,
  which contains a `string` that represents the json to include in the set of
  parameters to be sent to the broker. The `parametersFrom` field is a list which
  supports multiple sources referenced per `spec`.

You may use either, or both, of these fields as needed.

If multiple sources in `parameters` and `parametersFrom` blocks are specified,
the final payload is a result of merging all of them at the top level.
If there are any duplicate properties defined at the top level, the specification
is considered to be invalid, the further processing of the `ServiceInstance`/`ServiceBinding`
resource stops and its `status` is marked with error condition.

The format of the `spec` in YAML
```yaml
spec:
  ...
  parameters:
    name: value
  parametersFrom:
    - secretKeyRef:
        name: my-secret
        key: secret-parameter
```

The format of the `spec` in JSON
```json
"spec": {
  "parameters": {
    "name": "value"
  },
  "parametersFrom": {
    "secretKeyRef": {
      "name": "my-secret",
      "key": "secret-parameter"
    }
  }
}
```
The `secret` with the `secret-parameter`- named key:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
type: Opaque
stringData:
  secret-parameter:
    '{
      "password": "letmein"
    }'
```
The final JSON payload to send to the broker:
```json
{
  "name": "value",
  "password": "letmein"
}
```

You can list multiple parameters in the `secret`. To do so, separate "key": "value" pairs with commas as in this example:
```yaml
  secret-parameter:
    '{
      "password": "letmein",
      "key2": "value2",
      "key3": "value3"
    }'
```
[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes).

### Managing Access
By default, the SAP BTP operator has cluster-wide permissions.<br>You can also limit them to one or more namespaces; for this, you need to set the following two helm parameters:

```
--set manager.allow_cluster_access=false
--set manager.allowed_namespaces={namespace1, namespace2..}
```
**Note:**<br> If `allow_cluster_access` is set to true, then `allowed_namespaces` parameter is ignored.


## SAP BTP kubectl Plugin (Experimental)
The SAP BTP kubectl plugin extends kubectl with commands for getting the available services in your SAP BTP account by
using the access credentials stored in the cluster.

### Prerequisites
- [jq](https://stedolan.github.io/jq/)

### Limitations
- The SAP BTP kubectl plugin is currently based on `bash`. If you're using Windows, you should utilize the SAP BTP plugin commands from a linux shell (e.g. [Cygwin](https://www.cygwin.com/)).

### Installation
- Download `https://github.com/SAP/sap-btp-service-operator/releases/download/<release>/kubectl-sapbtp`
- Move the executable file to any location in your `PATH`

The list of available releases: [sapbtp-operator releases](https://github.com/SAP/sap-btp-service-operator/releases)

### Usage
```
  kubectl sapbtp marketplace -n <namespace>
  kubectl sapbtp plans -n <namespace>
  kubectl sapbtp services -n <namespace>
```

Use the `namespace` parameter to specify the location of the secret containing the SAP BTP access credentials.  
Usually it is the namespace in which you installed the operator.
If not specified, the `default` namespace is used.

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes).

## Credentials Rotation
To enable automatic credentials rotation, you need to set the following parameters of the `credentialsRotationPolicy` field in the `spec` field of the `ServiceBinding` resource:

- `enabled` - Whether the credentials rotation option is enabled. Default value is false. 
- `rotationFrequency` - Indicates the frequency at which the credentials rotation is performed. 
- `rotatedBindingTTL` - Indicates for how long to keep the rotated `ServiceBinding`.

Valid time units for `rotationFrequency` and `rotatedBindingTTL` are: "ns", "us" or ("µs"), "ms", "s", "m", "h".</br></br>
`status.lastCredentialsRotationTime` indicates the last time the `ServiceBinding` secret was rotated.</br>
Please note that `credentialsRotationPolicy` evaluated and executed during [control loop](https://kubernetes.io/docs/concepts/architecture/controller/) which runs on every update or during
full reconciliation process.

During the transition period, there are two (or more) `ServiceBinding`: the original and the rotated one (holds the `services.cloud.sap.com/stale` label), which is deleted once the `rotatedBindingTTL` duration elapses.</br></br>
You can also choose the `services.cloud.sap.com/forceRotate` annotation (value doesn't matter), upon which immediate credentials rotation is performed. Note that the prerequisite for the force action is that credentials rotation `enabled` field is set to true.).

**Note:**<br> It isn't possible to enable automatic credentials rotation to an already-rotated `ServiceBinding` (with the `services.cloud.sap.com/stale` label).

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Multitenancy
You can configure the SAP BTP service operator to work with more than one subaccount in the same Kubernetes cluster. This means that different namespaces can be connected to different subaccounts.
The association between a namespace and a subaccount is based on a different set of credentials configured for different namespaces.

To connect the namespace to a subaccount, you first have to obtain the [access credentials](#setup) for the SAP BTP service operator and then maintain them in a secret that is specific for that namespace.

There are two options to maintain namespace-specific credentials, and they differ between default and TLS-based access credentials types:

### Default Access Credentials
- Define a secret named `sap-btp-service-operator` in the namespace. `ServiceInstance` and `ServiceBinding` that are applied in the namespace will belong to the subaccount from which the credentials were issued.  
- Define different secrets for different namespaces in a [centrally managed namespace](./sapbtp-operator-charts/templates/configmap.yml), following the secret naming convention: `sap-btp-service-operator-<namespace>`.
#### Namespace Secret Structure
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sap-btp-service-operator
  namespace: <namespace>
type: Opaque
data:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

### TLS-Based Access Credentials
- Define a secret pair named `sap-btp-service-operator` and `sap-btp-service-operator-tls`  in the namespace. `ServiceInstance` and `ServiceBinding` that are applied in the namespace will belong to the subaccount from which the credentials were issued.  
- Define different secrets for different namespaces in a [centrally managed namespace](./sapbtp-operator-charts/templates/configmap.yml), following the secret naming convention: `sap-btp-service-operator-<namespace>` and `sap-btp-service-operator-tls-<namespace>` . For more information, see [tls secret](./sapbtp-operator-charts/templates/secret-tls.yml).
#### Namespace Secrets Structure
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sap-btp-service-operator
  namespace: <namespace>
type: Opaque
data:
  clientid: "<clientid>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sap-btp-service-operator-tls
  namespace: <namespace>
type: kubernetes.io/tls
data:
  tls.crt: <crt> #base64 encoded
  tls.key: <key> #base64 encoded
```

**Notes:**
- If none of the those mentioned above options are set, `sap-btp-service-operator` secret of a release namespace is used.<br>
  See step 4 of the [Setup](#setup) section.
  
[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Troubleshooting and Support

  #### Cannot Create a Service Binding for Service Instance in `Delete Failed` State 

  The deletion of my service instance failed. To fix the failure, I have to create a service binding, but I can't do this because the instance is in the `Delete  Failed` state.
 
  **Solution** 
 
 To overcome this issue, use the `force_k8s_binding` query param when you create a service binding and set it to `true` (`force_k8s_binding=true`). You can do & this   either with the Service Manager Control CLI (smctl) [bind](https://help.sap.com/docs/SERVICEMANAGEMENT/09cc82baadc542a688176dce601398de/f53ff2634e0a46d6bfc72ec075418dcd.html) command or 'Create a Service Binding' [Service Manager API](https://api.sap.com/api/APIServiceManagment/resource).

  smctl Example

  >   ```bash
  >   smctl bind INSTANCE_NAME BINDING_NAME --param force_k8s_binding=true
  >   ```

<br>
  Once you've finished working on the service instance, delete it by running the following command:


  >   ```bash
  >   smctl unbind INSTANCE_NAME BINDING_NAME --param force_k8s_binding=true
  >   ```
  **Note:** `force_k8s_binding` is supported only for the Kubernetes instances that are in `Delete Failed` state.<br>

You're welcome to raise issues related to feature requests, bugs, or give us general feedback on this project's GitHub Issues page. 
The SAP BTP service operator project maintainers will respond to the best of their abilities. 

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Uninstalling the Operator

Before you uninstall the operator, we recommend you manually delete all associated service instances and bindings. This way, you'll ensure all data stored with service instances and bindings are properly taken care of. Instances and bindings that were not manually deleted will be automatically deleted once you start the uninstallation process.

To uninstall the operator, run the following command:
`helm uninstall <release name> -n <name space>`

Example: 

 >   ```
  >   helm uninstall sap-btp-operator -n sap-btp-operator

#### Responses

   - `release <release name> uninstalled` - The operator has been successfully uninstalled

   - `Timed out waiting for condition` 
   
      - What happened?
      
        The deletion of instances and bindings takes more than 5 minutes, this happens when there is a large number of instances and bindings.

      - What to do:
     
        Wait for the job to finish and re-trigger the uninstall process.
        To check the job status, run `kubectl get jobs --namespace=<name space>` or log on to the cluster and check the job log.
        Note that you may have to repeat this step several times untill the un-install process has been successfully completed.
     
     
   - `job failed: BackoffLimitExceeded`
      
     -  What happened?
      
        One of the service instances or bindings could not be deleted.
     
      - What to do:
      
        First find the service instance or binding in question and fix it, then re-trigger the uninstalation. 

        To find it, log on to the cluster and check the pre-delete job, or check the logs by running the following two commands:
        
          - `kubectl get pods --all-namespaces| grep pre-delete`  - which gives you the list of all namespaces and jobs
          - `kubectl logs <job_name> --namespace=<name_space_name>` - where you specify the desired job and namespace
          
        Note that the pre-delete job is only visible for approximately one minute after the job execution is completed. 
  
        If you don't have an access to the pre-delete job, use kubectl to view details about the failed resource and check its status by running:
        
          - `kubectl describe <resource_type> <resource_name>` 
          
        Check for resources with the deletion timestamp to determine if it tried to be deleted. 


## Contributions
We currently do not accept community contributions. 

## License
This project is licensed under Apache 2.0 unless noted otherwise in the [license](./LICENSE) file.

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)
