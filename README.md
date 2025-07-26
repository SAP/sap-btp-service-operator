# SAP BTP Service Operator for Kubernetes

The SAP BTP Service Operator enables you to consume and manage SAP BTP services directly from your Kubernetes cluster using Kubernetes-native tools. It allows provisioning and managing service instances and bindings of SAP BTP services, enabling Kubernetes-native applications to access these services from within the cluster.

The operator is built on the Kubernetes Operator pattern, extending Kubernetes' capabilities to manage SAP BTP services as first-class resources.

## Table of Contents

- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Setup](#setup)
- [Managing Access](#managing-access)
- [Working with Multiple Subaccounts](#working-with-multiple-subaccounts)
  - [Subaccount for a Namespace](#subaccount-for-a-namespace)
  - [Subaccount for a ServiceInstance Resource](#subaccount-for-a-serviceinstance-resource)
  - [Secrets Precedence](#secrets-precedence)
- [Using the SAP BTP Service Operator](#using-the-sap-btp-service-operator)
  - [Service Instance](#service-instance)
  - [Service Binding](#service-binding)
    - [Formats of Service Binding Secrets](#formats-of-service-binding-secrets)
      - [Key-Value Pairs (Default)](#key-value-pairs-default)
      - [Credentials as JSON Object](#credentials-as-json-object)
      - [Credentials and Service Info as One JSON Object](#credentials-and-service-info-as-one-json-object)
      - [Custom Formats](#custom-formats)
    - [Service Binding Rotation](#service-binding-rotation)
  - [Passing Parameters](#passing-parameters)
- [Reference Documentation](#reference-documentation)
  - [Service Instance Properties](#service-instance-properties)
  - [Service Binding Properties](#service-binding-properties)
- [Uninstalling the Operator](#uninstalling-the-operator)
- [Troubleshooting and Support](#troubleshooting-and-support)
  - [Cannot Create a Binding Because Service Instance Is in Delete Failed State](#cannot-create-a-binding-because-service-instance-is-in-delete-failed-state)
  - [Cluster Is Unavailable, and I Still Have Service Instances and Bindings](#cluster-is-unavailable-and-i-still-have-service-instances-and-bindings)
  - [Restoring a Missing Custom Resource](#restoring-a-missing-custom-resource)
- [Contributions](#contributions)
- [License](#license)

## Architecture

The SAP BTP Service Operator acts as an intermediary between your Kubernetes cluster and the SAP BTP Service Manager, facilitating the consumption of SAP BTP services by:

- **Communicating with Service Manager**: The operator interacts with the SAP BTP Service Manager, which uses the Open Service Broker API to communicate with various service brokers.
- **Negotiating Service Provisioning**: It negotiates the initial provisioning of SAP BTP service instances for Kubernetes applications via the Service Manager.
- **Retrieving Credentials**: It retrieves access credentials for applications to use the managed services.

The operator uses a **Custom Resource Definitions (CRDs)**-based architecture, allowing you to define and manage SAP BTP service instances and bindings using Kubernetes-native YAML manifests.

## Prerequisites

Before starting, ensure the following are in place:

- **SAP BTP Global Account and Subaccount**: You need an SAP BTP global account with a subaccount for consuming services.
- **Familiarity with SAP Service Manager**: Beneficial for understanding service management.
- **Kubernetes Cluster**: Version 1.17 or higher.
- **kubectl**: Command-line tool version 1.17 or higher.
- **helm**: Package manager version 3.0 or higher.

## Setup

Follow these steps to set up the SAP BTP Service Operator in your Kubernetes cluster:

1. **Install cert-manager**:
   - For operator releases **v0.1.18 or higher**, use **cert-manager v1.6.0 or higher**.
   - For operator releases **v0.1.17 or lower**, use **cert-manager lower than v1.6.0**.

2. **Obtain Access Credentials for the SAP BTP Service Operator**:
   - Create an instance of the SAP Service Manager service (technical name: `service-manager`) with the plan `service-operator-access`.
     - **Note**: If the plan is not visible, entitle your subaccount for the SAP Service Manager service. See [Configure Entitlements and Quotas for Subaccounts](https://help.sap.com/docs/btp).
     - For details, refer to [Creating Service Instances Using the SAP BTP Cockpit](https://help.sap.com/docs/btp) or [Creating Service Instances Using BTP CLI](https://help.sap.com/docs/btp).
   - Create a binding to the service instance. See [Creating Service Bindings Using the SAP BTP Cockpit](https://help.sap.com/docs/btp) or [Creating Service Bindings Using BTP CLI](https://help.sap.com/docs/btp).
   - Retrieve the generated access credentials from the binding.

   **Example of Default Binding Object** (no credentials type specified):
   ```json
   {
       "clientid": "xxxxxxx",
       "clientsecret": "xxxxxxx",
       "url": "https://mysubaccount.authentication.eu10.hana.ondemand.com",
       "xsappname": "b15166|service-manager!b1234",
       "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
   }
   ```

   **Example of Binding Object with X.509 Certificate**:
   ```json
   {
       "clientid": "xxxxxxx",
       "certificate": "-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n",
       "key": "-----BEGIN RSA PRIVATE KEY-----...-----END RSA PRIVATE KEY-----\n",
       "certurl": "https://mysubaccount.authentication.cert.eu10.hana.ondemand.com",
       "xsappname": "b15166|service-manager!b1234",
       "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
   }
   ```

3. **Add SAP BTP Service Operator Chart Repository**:
   ```bash
   helm repo add sap-btp-operator https://sap.github.io/sap-btp-service-operator
   ```

4. **Deploy the SAP BTP Service Operator**:
   - **Note**: For registered clusters usingr using Service Catalog (svcat) and Service Manager agent (for migration), add `--set cluster.id=<clusterID>` to the deployment script. See [Migration to SAP BTP Service Operator](https://help.sap.com/docs/btp).
   - **Example Deployment with Default Access Credentials**:
     ```bash
     helm upgrade --install <release-name> sap-btp-operator/sap-btp-operator \
         --create-namespace \
         --namespace=sap-btp-operator \
         --set manager.secret.clientid=<clientid> \
         --set manager.secret.clientsecret=<clientsecret> \
         --set manager.secret.sm_url=<sm_url> \
         --set manager.secret.tokenurl=<auth_url>
     ```
   - **Example Deployment with X.509 Certificate**:
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

5. **BTP Access Secret Structure**:
   - The credentials are stored in a Kubernetes Secret named `sap-btp-service-operator` in the `sap-btp-operator` namespace, used by the operator to communicate with the SAP BTP subaccount.
   - Fields: `clientid`, `clientsecret`, `sm_url`, `tokenurl`, `tokenurlsuffix`, `tls.crt`, `tls.key`.
   - **Note**: To rotate credentials, create a new binding for the `service-operator-access` instance, re-run the setup script with new credentials, and delete the old binding.

## Managing Access

By default, the SAP BTP Service Operator has **cluster-wide permissions**. To limit permissions to specific namespaces, set these Helm parameters during deployment:

```bash
--set manager.allow_cluster_access=false
--set manager.allowed_namespaces={namespace1,namespace2,...}
```

**Note**: If `allow_cluster_access` is `true`, `allowed_namespaces` is ignored.

## Working with Multiple Subaccounts

By default, a Kubernetes cluster is associated with a single SAP BTP subaccount. The operator supports **multi-subaccount configurations** via:

- **Namespace-based Mapping**: Connect different namespaces to separate subaccounts using dedicated credentials.
- **Explicit Instance-level Mapping**: Define the subaccount for each service instance, regardless of namespace.

Both methods use secrets in a centrally managed namespace (set via `.Values.manager.management_namespace`, defaulting to the installation namespace).

### Subaccount for a Namespace

Associate a namespace with a subaccount by creating a Secret named `<namespace-name>-sap-btp-service-operator` in the centrally managed namespace.

**Default Access Credentials Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <namespace-name>-sap-btp-service-operator
  namespace: <centrally-managed-namespace>
type: Opaque
stringData:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

**mTLS Access Credentials Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <namespace-name>-sap-btp-service-operator
  namespace: <centrally-managed-namespace>
type: Opaque
stringData:
  clientid: <clientid>
  tls.crt: <certificate>
  tls.key: <key>
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

### Subaccount for a ServiceInstance Resource

Deploy service instances from different subaccounts in the same namespace by:

1. Storing access credentials in separate Secrets in the centrally managed namespace.
2. Referencing the Secret in the `ServiceInstance` resource via `btpAccessCredentialsSecret`.

**Default Access Credentials Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <my-secret>
  namespace: <centrally-managed-namespace>
type: Opaque
stringData:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

**mTLS Access Credentials Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <my-secret>
  namespace: <centrally-managed-namespace>
type: Opaque
stringData:
  clientid: <clientid>
  tls.crt: <certificate>
  tls.key: <key>
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

**ServiceInstance with Explicit Subaccount**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceInstance
metadata:
  name: sample-instance-1
spec:
  serviceOfferingName: service-manager
  servicePlanName: subaccount-audit
  btpAccessCredentialsSecret: mybtpsecret # Secret in management namespace
```

### Secrets Precedence

The operator searches for credentials in this order:

1. Explicit secret in `ServiceInstance` (`btpAccessCredentialsSecret`).
2. Default namespace secret (`<namespace-name>-sap-btp-service-operator` in centrally managed namespace).
3. Default cluster secret (`sap-btp-service-operator` in operator’s installation namespace).

## Using the SAP BTP Service Operator

### Service Instance

Create a `ServiceInstance` custom resource to provision an SAP BTP service:

```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceInstance
metadata:
  name: my-service-instance
spec:
  serviceOfferingName: sample-service  # SAP BTP service offering
  servicePlanName: sample-plan        # Plan of the service offering
  externalName: my-service-btp-name   # Optional: Name in SAP BTP, defaults to metadata.name
  parameters:                         # Optional: Additional configuration
    key1: val1
    key2: val2
```

- **`<offering>`**: Name of the SAP BTP service. See [Service Marketplace](https://help.sap.com/docs/btp) for available services (use Environment filter for Kubernetes-relevant offerings).
- **`<plan>`**: Plan of the selected service offering.

Apply the resource:
```bash
kubectl apply -f path/to/my-service-instance.yaml
```

Check status:
```bash
kubectl get serviceinstances
```

**Example Output**:
```
NAME                  OFFERING          PLAN        STATUS    AGE
my-service-instance   sample-service    sample-plan Created   44s
```

### Service Binding

Create a `ServiceBinding` custom resource to obtain access credentials for an application, stored in a Secret.

**ServiceBinding Structure**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance  # Kubernetes name of ServiceInstance
  externalName: my-binding-external    # Optional: Name in SAP BTP
  secretName: my-secret                # Optional: Secret name, defaults to metadata.name
  parameters:                          # Optional: Bind request parameters
    key1: val1
    key2: val2
```

Apply the resource:
```bash
kubectl apply -f path/to/my-binding.yaml
```

Verify status:
```bash
kubectl get servicebindings
```

**Example Output**:
```
NAME         INSTANCE              STATUS    AGE
my-binding   my-service-instance   Created   16s
```

Check Secret creation:
```bash
kubectl get secrets
```

**Example Output**:
```
NAME         TYPE     DATA   AGE
my-secret   Opaque   5      32s
```

See [Using Secrets](https://help.sap.com/docs/btp) for ways to use credentials in applications.

#### Formats of Service Binding Secrets

Secrets contain credentials from the broker and `ServiceInstance` attributes, with various formats based on `ServiceBinding` attributes.

##### Key-Value Pairs (Default)

Without specific attributes, the Secret uses key-value pairs:

**ServiceBinding**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance
```

**Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sample-binding
stringData:
  uri: https://my-service.authentication.eu10.hana.ondemand.com
  client_id: admin
  client_secret: ********
  instance_guid: your-sample-instance-guid # Service instance ID
  instance_name: sample-instance          # From external_name or metadata.name
  plan: sample-plan                      # Service plan name
  type: sample-service                   # Service offering name
```

##### Credentials as JSON Object

Use `secretKey` to store credentials as a JSON object:

**ServiceBinding**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance
  secretKey: myCredentials
```

**Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sample-binding
stringData:
  myCredentials:
    uri: https://my-service.authentication.eu10.hana.ondemand.com,
    client_id: admin,
    client_secret: ********
  instance_guid: your-sample-instance-guid
  instance_name: sample-binding
  plan: sample-plan
  type: sample-service
```

##### Credentials and Service Info as One JSON Object

Use `secretRootKey` to store credentials and `ServiceInstance` info as a JSON object:

**ServiceBinding**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance
  secretRootKey: myCredentialsAndInstance
```

**Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sample-binding
stringData:
  myCredentialsAndInstance:
    uri: https://my-service.authentication.eu10.hana.ondemand.com,
    client_id: admin,
    client_secret: ********,
    instance_guid: your-sample-instance-guid,
    instance_name: sample-instance-name,
    plan: sample-instance-plan,
    type: sample-instance-offering
```

##### Custom Formats

Use `secretTemplate` for custom Secret formats with Go templates:

**ServiceBinding with Custom Template**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance
  secretTemplate: |
    apiVersion: v1
    kind: Secret
    metadata:
      labels:
        service_plan: {{ .instance.plan }}
      annotations:
        instance: {{ .instance.instance_name }}
    stringData:
      USERNAME: {{ .credentials.client_id }}
      PASSWORD: {{ .credentials.client_secret }}
```

**Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  labels:
    service_plan: sample-plan
  annotations:
    instance: sample-instance
stringData:
  USERNAME: admin
  PASSWORD: ********
```

**ServiceBinding with Custom Metadata and secretKey**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance
  secretKey: myCredentials
  secretTemplate: |
    apiVersion: v1
    kind: Secret
    metadata:
      labels:
        service_plan: {{ .instance.plan }}
      annotations:
        instance: {{ .instance.instance_name }}
```

**Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  labels:
    service_plan: sample-plan
  annotations:
    instance: sample-instance
stringData:
  myCredentials:
    uri: https://my-service.authentication.eu10.hana.ondemand.com,
    client_id: admin,
    client_secret: ********
  instance_guid: your-sample-instance-guid
  instance_name: sample-binding
  plan: sample-plan
  type: sample-service
```

**Template Attributes**:
| Reference                  | Description                          |
|----------------------------|--------------------------------------|
| `instance.instance_guid`   | Service instance ID                  |
| `instance.instance_name`   | Service instance name                |
| `instance.plan`            | Service plan name                    |
| `instance.type`            | Service offering name                |
| `credentials.attributes`   | Credentials (service-specific)        |

**Note**: `secretTemplate` takes precedence over predefined formats if `stringData` is customized.

#### Service Binding Rotation

Enhance security by automatically rotating service binding credentials, keeping old credentials active for a transition period.

**Enabling Automatic Rotation**:
Use `credentialsRotationPolicy` in the `ServiceBinding` spec:

| Parameter            | Type   | Description                                              | Valid Values         |
|----------------------|--------|----------------------------------------------------------|----------------------|
| `enabled`            | bool   | Enables/disables automatic rotation                      | `true`, `false`      |
| `rotationFrequency`  | string | Time interval between rotations                          | `m` (minute), `h` (hour) |
| `rotatedBindingTTL`  | string | Duration to keep old ServiceBinding before deletion      | `m` (minute), `h` (hour) |

**Note**: `credentialsRotationPolicy` does not manage credential validity (service-specific).

**Rotation Process**:
- Evaluated during control loop (on binding update or full reconciliation).
- Actual rotation occurs in the next reconciliation loop.

**Immediate Rotation**:
Add `services.cloud.sap.com/forceRotate: "true"` annotation to trigger immediate rotation (requires `enabled: true`).

**Example**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance
  credentialsRotationPolicy:
    enabled: true
    rotatedBindingTTL: 48h
    rotationFrequency: 600h
```

**After Rotation**:
- Secret updates with new credentials.
- Old credentials are stored in a temporary Secret (`original-secret-name<variable>-guid`) until `rotatedBindingTTL` expires.
- Check last rotation via `status.lastCredentialsRotationTime`.

**Limitations**:
- Automatic rotation is not supported for backup ServiceBindings (marked with `services.cloud.sap.com/stale`).

### Passing Parameters

Set input parameters using `parameters` and `parametersFrom` in `ServiceInstance` or `ServiceBinding` spec:

- **parameters**: Properties sent to the broker as-is (YAML/JSON converted to JSON).
- **parametersFrom**: List of Secrets with JSON-formatted parameters.
- **watchParametersFromChanges**: If `true`, Secret changes trigger `ServiceInstance` updates (default: `false`, only for `ServiceInstance`).

**Example YAML**:
```yaml
spec:
  parameters:
    name: value
  parametersFrom:
    - secretKeyRef:
        name: my-secret
        key: secret-parameter
```

**Example JSON**:
```json
{
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
}
```

**Secret Example**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
type: Opaque
stringData:
  secret-parameter: '{"password": "password"}'
```

**Final Payload to Broker**:
```json
{
  "name": "value",
  "password": "password"
}
```

**Multiple Parameters in Secret**:
```yaml
secret-parameter: '{"password": "password", "key2": "value2", "key3": "value3"}'
```

**Note**: Duplicate top-level properties in `parameters` and `parametersFrom` cause an error, stopping processing with an error status.

## Reference Documentation

### Service Instance Properties

**Spec**:
| Parameter                     | Type     | Description                                                                 |
|-------------------------------|----------|-----------------------------------------------------------------------------|
| `serviceOfferingName*`        | string   | Name of the SAP BTP service offering.                                       |
| `servicePlanName*`            | string   | Plan to use for the service instance.                                       |
| `servicePlanID`               | string   | Plan ID if offering and plan name are ambiguous.                            |
| `externalName`                | string   | Name in SAP BTP, defaults to `metadata.name`.                               |
| `parameters`                  | []object | Additional configuration parameters (service-specific).                      |
| `parametersFrom`              | []object | List of sources to populate parameters.                                     |
| `watchParametersFromChanges`  | bool     | Triggers instance update on Secret changes (default: `false`).               |
| `customTags`                  | []string | Custom tags copied to ServiceBinding Secret as `tags`.                      |
| `userInfo`                    | object   | Info about the user who last modified the instance.                         |
| `shared*`                     | bool     | Shared state (`true`, `false`, or `nil` = `false`).                         |
| `btpAccessCredentialsSecret`  | string   | Secret name for SAP BTP access credentials. See [Working with Multiple Subaccounts](#working-with-multiple-subaccounts). |

**Status**:
| Parameter        | Type       | Description                                                                 |
|------------------|------------|-----------------------------------------------------------------------------|
| `instanceID`     | string     | Service instance ID in SAP Service Manager.                                 |
| `operationURL`   | string     | URL of the current operation.                                               |
| `operationType`  | string     | Current operation type (`CREATE`, `UPDATE`, `DELETE`).                      |
| `conditions`     | []condition | Status conditions:                                                         |
|                  |            | - `Ready`: `true` if instance is usable.                                   |
|                  |            | - `Failed`: `true` on operation failure (details in condition message).     |
|                  |            | - `Succeeded`: `true` on operation success, `false` if in progress.         |
|                  |            | - `Shared`: `true` if sharing succeeded, `false` if not shared.             |
| `tags`           | []string   | Tags from service catalog, copied to ServiceBinding Secret as `tags`.       |

**Annotations**:
| Parameter                           | Type               | Description                                                                 |
|-------------------------------------|--------------------|-----------------------------------------------------------------------------|
| `services.cloud.sap.com/preventDeletion` | map[string]string | Set to `"true"` to prevent deletion; remove or set to `"false"` to allow.   |

### Service Binding Properties

**Spec**:
| Parameter                  | Type   | Description                                                                 |
|----------------------------|--------|-----------------------------------------------------------------------------|
| `serviceInstanceName*`     | string | Kubernetes name of the ServiceInstance to bind.                             |
| `serviceInstanceNamespace` | string | Namespace of the ServiceInstance (defaults to binding’s namespace).         |
| `externalName`             | string | Name in SAP BTP (defaults to `metadata.name`).                             |
| `secretName`               | string | Secret name for credentials (defaults to `metadata.name`).                  |
| `secretKey`                | string | Key for credentials in Secret (stores credentials as JSON).                 |
| `secretRootKey`            | string | Key for credentials and instance info in Secret (stores as JSON).           |

## Uninstalling the Operator

Manually delete all service instances and bindings before uninstalling to ensure proper data cleanup. Unmanaged resources are automatically deleted during uninstallation.

**Command**:
```bash
helm uninstall <release-name> -n <namespace>
```

**Example**:
```bash
helm uninstall sap-btp-operator -n sap-btp-operator
```

**Response**:
```
release <release-name> uninstalled
```

**Troubleshooting Uninstallation**:

- **Timed out waiting for condition**:
  - **Cause**: Deletion of many instances/bindings takes >5 minutes.
  - **Solution**: Wait for the job to finish, check status with `kubectl get jobs --namespace=<namespace>`, and retry uninstallation.
- **Job failed: BackoffLimitExceeded**:
  - **Cause**: A service instance/binding could not be deleted.
  - **Solution**: Identify and fix the resource using:
    ```bash
    kubectl get pods --all-namespaces | grep pre-delete
    kubectl logs <job_name> --namespace=<namespace>
    kubectl describe <resource_type> <resource_name>
    ```
    Check for resources with deletion timestamps, then retry uninstallation.

## Troubleshooting and Support

### Cannot Create a Binding Because Service Instance Is in Delete Failed State

**Issue**: Cannot create a binding due to a service instance in `Delete Failed` state.

**Solution**:
Use `force_k8s_binding=true` with the Service Manager Control CLI (`smctl`):
```bash
smctl bind INSTANCE_NAME BINDING_NAME --param force_k8s_binding=true
```

Delete the binding afterward:
```bash
smctl unbind INSTANCE_NAME BINDING_NAME --param force_k8s_binding=true
```

**Note**: Do not use `service-operator-access` plan credentials. This applies only to Kubernetes instances in `Delete Failed` state.

### Cluster Is Unavailable, and I Still Have Service Instances and Bindings

**Issue**: Cannot delete instances/bindings due to an unavailable cluster.

**Solution**:
Use the Service Manager API to clean up (requires `subaccount-admin` plan credentials, not `service-operator-access`):

**Request**:
```
DELETE /v1/platforms/{platformID}/clusters/{clusterID}
```

**Parameters**:
| Parameter   | Type   | Description                                                                 |
|-------------|--------|-----------------------------------------------------------------------------|
| `platformID`| string | ID of the platform (`service-operator-access` instance ID).                  |
| `clusterID` | string | Cluster ID from Setup step 4, or from GET service instance/binding API.      |

**Response**:
| Status Code | Description                     |
|-------------|---------------------------------|
| 202         | Accepted for processing         |
| 404         | Platform or cluster not found   |
| 429         | Too Many Requests (rate limit)  |

**Headers**:
- `Location`: Path to operation status. See [Service Manager Operation API](https://help.sap.com/docs/btp).
- `Retry-After`: Time (seconds) to wait before retrying (for 429).

**Warning**: Use only for cleanup of unavailable clusters to avoid resource leftovers in active clusters.

### Restoring a Missing Custom Resource

**Issue**: Service instance/binding exists in SAP BTP but not as a custom resource in the Kubernetes cluster.

**Background**:
A lost custom resource (CR) can be restored by recreating it with the same name, namespace, and cluster ID, reconnecting to the existing SAP BTP resource without provisioning a new one.

**Steps**:
1. **Retrieve CR Details**:
   - Access the service instance/binding in SAP BTP.
   - Note the CR name and Kubernetes namespace.
2. **Recreate the CR**:
   - Use the original YAML manifest, ensuring the same name and namespace.
   - Apply with:
     ```bash
     kubectl apply -f <your_cr_manifest.yaml>
     ```
3. **Verify**:
   - Check CR creation:
     ```bash
     kubectl get <your_cr_kind> <your_cr_name> -n <your_namespace>
     ```
   - Confirm SAP BTP recognizes the connection.
   - Verify cluster ID matches (from SAP BTP Cockpit or BTP CLI). Reconfigure if mismatched.

**Support**:
Raise issues, feature requests, or feedback on the [GitHub Issues page](https://github.com/sap/sap-btp-service-operator).

## Contributions

Community contributions are currently not accepted.

## License

Licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0), unless noted otherwise in the license file.



