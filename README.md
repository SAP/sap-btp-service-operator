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
  * [Managing access](#managing-access)
  * [Working with Multiple Subaccounts](#working-with-multiple-subaccounts)
* [Using the SAP BTP Service Operator](#using-the-sap-btp-service-operator)
    * [Service Instance](#service-instance)
    * [Service Binding](#service-binding)
      * [Formats of output Secret](#formats-of-output-secret)
      * [Automatic Service Binding Rotation](#automatic-service-binding-rotation)
    * [Passing parameters](#passing-parameters)
* [Reference Documentation](#reference-documentation)
    * [Service Instance properties](#Service-Instance-properties)
    * [Service Binding properties](#service-binding-properties)
* [Uninstalling the Operator](#uninstalling-the-operator)
* [Troubleshooting and Support](#troubleshooting-and-support)

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
        --set manager.secret.tokenurl=<auth_url>
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
        --set manager.secret.tokenurl=<auth_url>
    ```

The credentials which are provided during the installation are stored in a secret named 'sap-btp-service-operator', in the 'sap-btp-operator' namespace.
These credentials are used by the BTP service operator to communicate with the SAP BTP subaccount.

<details>
<summary> BTP Access Secret Structure </summary>

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sap-btp-service-operator
  namespace: sap-btp-operator
type: Opaque
data:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```
</details>

**Note:**<br> In order to rotate the credentials between the BTP service operator and Service Manager, you have to create a new binding for the service-operator-access service instance, and then to execute the setup script again, with the new set of credentials. Afterwards you can delete the old binding.

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Managing Access
By default, the SAP BTP operator has cluster-wide permissions.<br>You can also limit them to one or more namespaces; for this, you need to set the following two helm parameters:

```
--set manager.allow_cluster_access=false
--set manager.allowed_namespaces={namespace1, namespace2..}
```
**Note:**<br> If `allow_cluster_access` is set to true, then `allowed_namespaces` parameter is ignored.

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Working with Multiple Subaccounts

By default, a Kubernetes cluster is associated with a single subaccount (as described in step 4 of the [Setup](#setup) section). 
Consequently, any service instance created within any namespace will be provisioned in that subaccount.

However, the SAP BTP service operator also supports multi-subaccount configurations in a single cluster. This is achieved through:

- Namespace-based mapping: Connect different namespaces to separate subaccounts. This approach leverages dedicated credentials configured for each namespace.
  
- Explicit instance-level mapping: Define the specific subaccount for each service instance, regardless of the namespace context.

  Both can be achieved through dedicated secrets managed in the centrally-managed namespace. Choosing the most suitable approach depends on your specific needs and application architecture.

**Note:**
The system's centrally-managed namespace is set by the value in `.Values.manager.management_namespace`. You can provide this value during installation (refer to step 4 in the [Setup](#setup) section).
If you don't specify this value, the system will use the installation namespace as the default.

### Subaccount For a Namespace

To associate namespace to a specific subaccount you maintain the access credentials to the subaccount in a secret which is dedicated to a specific namespace.
Define a secret named: `<namespace-name>-sap-btp-service-operator` in the Centrally Managed Namespace.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <namespace-name>-sap-btp-service-operator
  namespace: <centrally managed namespace>
type: Opaque
data:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

### Subaccount for a ServiceInstance Resource

You can deploy service instances belonging to different subaccounts within the same namespace. To achieve this, follow these steps:

1. Store access credentials: Securely store the access credentials for each subaccount in separate secrets within the centrally-managed namespace.
2. Specify subaccount per service: In the `ServiceInstance` resource, use the `btpAccessCredentialsSecret` property to reference the specific secret containing the relevant subaccount's credentials. This explicitly tells the operator which subaccount to use for provisioning the service instance.


#### Define a new secret
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mybtpsecret
  namespace: <centrally managed namespace>
type: Opaque
data:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

#### Configure the secret name in the `ServiceInstance` resource within the property `btpAccessCredentialsSecret`:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceInstance
metadata:
  name: sample-instance-1
spec:
  serviceOfferingName: service-manager
  servicePlanName: subaccount-audit
  btpAccessCredentialsSecret: mybtpsecret
```

##### Secrets Presedence
SAP BTP service operator search for the credentials in the following order:
1. Explicit secret defined in the `ServiceInstance`
2. Default namespace secret
3. Default cluster secret

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Using the SAP BTP Service Operator

#### Service Instance

1.  To create an instance of a service offered by SAP BTP, first create a `ServiceInstance` custom-resource file:

```yaml
    apiVersion: services.cloud.sap.com/v1
    kind: ServiceInstance
    metadata:
        name: my-service-instance
    spec:
        serviceOfferingName: sample-service
        servicePlanName: sample-plan
        externalName: my-service-btp-name
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

#### Service Binding

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
  secretName: my-secret
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

##### Formats of output Secret

###### Key- Value Pairs (Default)
The binding object includes credentials returned from the broker and service instance info presented as key-value pairs.
```bash
#Credentials
uri: https://my-service.authentication.eu10.hana.ondemand.com
username: admin
password: ********

#Service instance info
instance_guid: <instance_guid> // The service instance ID
instance_name: my-service-btp-name // Taken from the service instance external_name field if set. Otherwise from metadata.name
plan: sample-plan // The service plan name                
type: sample-service  // The service offering name
```

###### Credentials as JSON Object
To show credentials returned from the broker as a JSON object, use the 'secretKey' attribute in the service binding spec.

The value of 'secretKey' is the name of the key that stores the credentials in JSON format.

```bash
#Credentials
your-secretKey-value:
{
  uri: https://my-service.authentication.eu10.hana.ondemand.com
  username: admin
  password: ********
}

#Service Instance info
instance_guid: <instance_guid> // The service instance ID
instance_name: my-service-btp-name // Taken from the service instance external_name field if set. Otherwise from metadata.name 
plan: sample-plan // The service plan name
type: sample-service // The service offering name
```

###### Credentials and Service Info as One JSON Object
To show both credentials returned from the broker and service instance info as a JSON object, use the 'secretRootKey' attribute in the service binding spec.

The value of 'secretRootKey' is the name of the key that stores both credentials and serivce instance info in JSON format.

```bash
your-secretRootKey-value:
{
    #Credentials
    uri: https://my-service.authentication.eu10.hana.ondemand.com
    username: admin
    password: ********
    
    #Service Instance info
    instance_guid: <instance_guid> // The service instance id
    instance_name: my-service-btp-name // Taken from the service instance external_name field if set. Otherwise from metadata.name 
    plan: sample-plan // The service plan name
    type: sample-service // The service offering name
}
```

[Back to top](#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes)

## Automatic Service Binding Rotation

You can enhance security by rotating your service binding, ensuring that credentials are updated before they expire. Rotation involves generating new bindings with fresh credentials, while preserving and renaming the old ones for backup purposes until the transition is complete, after which they can be safely deleted.

To enable automatic credentials rotation, you need to set the following parameters of the `credentialsRotationPolicy` field in the `spec` field of the `ServiceBinding` resource:


| Parameter         | Type     | Description                                      |   Valid Values  |
|:-----------------|:---------|:----------------------------------------------------|:---------------------|
| `enabled` | bool | Whether the credentials rotation option is enabled.    |                                                                                                                                                          |
| `rotationFrequency` | string | Indicates the interval between credential rotations. The rotation will occur in the next reconciliation loop if the time that has passed since the `lastCredentialsRotationTime` is greater than the specified `rotationFrequency` interval. **Note that the actual frequency may be longer than the one you specified.**  See under **Important** below.   |   "ns", "us" or ("µs"), "ms", "s", "m", "h" |                                                                                                                                                                |
| `rotatedBindingTTL`   |  string | Specifies for how long to keep the rotated `ServiceBinding`.   |      "ns", "us" or ("µs"), "ms", "s", "m", "h" |   


**Recommendation** 

Set the TTL shorter than the rotation frequency. This gives you the opportunity to avoid the transition period timely inform your users of new credentials before the binding's validity expires. 



If your TTL is longer than the rotation frequency,  the following occurs:

There is a transition period during which the rotated binding is still valid. Namely, there are two (or more) `ServiceBinding` objects: the original and the rotated one (holds the `services.cloud.sap.com/stale` label). 
The rotated one is deleted once the `rotatedBindingTTL` duration elapses.

The TTL provides an opportunity to inform your users of new credentials before the binding's validity expires. To optimize this process, set the TTL to be shorter than the rotation frequency. This eliminates the need to back up two sets of credentials simultaneously.

**Important**

- The `credentialsRotationPolicy` is evaluated and executed during the [control loop](https://kubernetes.io/docs/concepts/architecture/controller/) which runs on every update or during
  a full reconciliation process. This means that the actual rotation time may potentially be greater than the specified duration in 'rotationFrequency'.
  
-It isn't possible to enable automatic credentials rotation to an already-rotated `ServiceBinding` (with the `services.cloud.sap.com/stale` label).

**Note**

While credentials are rotated, they are still stored in the original secret created for the first binding.

**Tip** 

To see when the service binding was last rotated, refer to the `status.lastCredentialsRotationTime`.

You can also choose the `services.cloud.sap.com/forceRotate` annotation (value doesn't matter), upon which immediate credentials rotation is performed. Note that the prerequisite for the force action is that credentials rotation `enabled` field is set to true.).


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

## Reference Documentation

### Service Instance properties
#### Spec
| Parameter         | Type     | Description                                                                                                                                                                                                       |
|:-----------------|:---------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| serviceOfferingName`*` | `string` | The name of the SAP BTP service offering.                                                                                                                                                                         |
| servicePlanName`*` | `string` | The plan to use for the service instance.                                                                                                                                                                         |
| servicePlanID   |  `string`  | The plan ID in case service offering and plan name are ambiguous.                                                                                                                                                 |
| externalName       | `string` | The name for the service instance in SAP BTP, defaults to the instance `metadata.name` if not specified.                                                                                                          |
| parameters       | `[]object` | Some services support the provisioning of additional configuration parameters during the instance creation.<br/>For the list of supported parameters, check the documentation of the particular service offering. |
| parametersFrom | `[]object` | List of sources to populate parameters.                                                                                                                                                                           |
| customTags | `[]string` | List of custom tags describing the ServiceInstance, will be copied to `ServiceBinding` secret in the key called `tags`.                                                                                           |
| userInfo | `object` | Contains information about the user that last modified this service instance.                                                                                                                                     |
| shared |  `*bool`   | The shared state. Possible values: true, false, or nil (value was not specified, counts as "false").                                                                                                              |
| btpAccessCredentialsSecret |  `string`   | Name of a secret which contain access credentials for the SAP BTP service operator. see [Working with Multiple Subaccounts](#Working-with-multiple-subaccounts)                                                                                 |


#### Status
| Parameter         | Type     | Description                                                                                                   |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------|
| instanceID   | `string` | The service instance ID in SAP Service Manager service.  |
| operationURL | `string` | The URL of the current operation performed on the service instance.  |
| operationType   |  `string`| The type of the current operation. Possible values are CREATE, UPDATE, or DELETE. |
| conditions       |  `[]condition`   | An array of conditions describing the status of the service instance.<br/>The possible condition types are:<br>- `Ready`: set to `true`  if the instance is ready and usable<br/>- `Failed`: set to `true` when an operation on the service instance fails.<br/> In the case of failure, the details about the error are available in the condition message.<br>- `Succeeded`: set to `true` when an operation on the service instance succeeded. In case of `false` operation considered as in progress unless `Failed` condition exists.<br>- `Shared`: set to `true` when sharing of the service instance succeeded. set to `false` when unsharing of the service instance succeeded or when service instance is not shared. |
| tags       |  `[]string`   | Tags describing the ServiceInstance as provided in service catalog, will be copied to `ServiceBinding` secret in the key called `tags`.|

#### Anotations
| Parameter         | Type                 | Description                                                                                                                                                                                                                         |
|:-----------------|:---------------------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| services.cloud.sap.com/preventDeletion   | `map[string] string` | You can prevent deletion of any service instance by adding the following annotation: services.cloud.sap.com/preventDeletion : "true". To enable back the deletion of the instance, either remove the annotation or set it to false. |

### Service Binding properties
#### Spec
| Parameter             | Type       | Description                                                                                                                                                                                                                                                                                                                              |
|:-----------------|:---------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| serviceInstanceName`*`   | `string`   | The Kubernetes name of the service instance to bind, should be in the namespace of the binding.                                                                                                                                                                                                                                          |
| externalName       | `string`   | The name for the service binding in SAP BTP, defaults to the binding `metadata.name` if not specified.                                                                                                                                                                                                                                   |
| secretName       | `string`   | The name of the secret where the credentials are stored, defaults to the binding `metadata.name` if not specified.                                                                                                                                                                                                                       |
| secretKey | `string`  | The secret key is a part of the Secret object, which stores service binding data (credentials) received from the broker. When the secret key is used, all the credentials are stored under a single key. This makes it a convenient way to store credentials data in one file when using volumeMounts. [Example](#formats-of-secret-objects)                                                                                                                                    |
| secretRootKey | `string`  | The root key is a part of the Secret object, which stores service binding data (credentials) received from the broker, as well as additional service instance information. When the root key is used, all data is stored under a single key. This makes it a convenient way to store data in one file when using volumeMounts. [Example](#formats-of-secret-objects) |
| parameters       |  `[]object`  | Some services support the provisioning of additional configuration parameters during the bind request.<br/>For the list of supported parameters, check the documentation of the particular service offering.                                                                                                                             |
| parametersFrom | `[]object` | List of sources to populate parameters.                                                                                                                                                                                                                                                                                                  |
| userInfo | `object`  | Contains information about the user that last modified this service binding.                                                                                                                                                                                                                                                             |
| credentialsRotationPolicy | `object`  | Holds automatic credentials rotation configuration.                                                                                                                                                                                                                                                                                      |
| credentialsRotationPolicy.enabled | `boolean`  | Indicates whether automatic credentials rotation are enabled.                                                                                                                                                                                                                                                                            |
| credentialsRotationPolicy.rotationFrequency | `duration`  | Specifies the frequency at which the binding rotation is performed.                                                                                                                                                                                                                                                                      |
| credentialsRotationPolicy.rotatedBindingTTL | `duration`  | Specifies the time period for which to keep the rotated binding.                                                                                                                                                                                                                                                                         |



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
