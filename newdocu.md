# SAP BTP Service Operator for Kubernetes

The SAP BTP Service Operator allows you to manage SAP Business Technology Platform (BTP) services directly from a Kubernetes cluster using Kubernetes-native tools. It simplifies provisioning and managing service instances and bindings, enabling applications to access SAP BTP services seamlessly.

The operator follows the Kubernetes Operator pattern, extending Kubernetes to treat SAP BTP services as native resources.

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

The SAP BTP Service Operator bridges your Kubernetes cluster and the SAP BTP Service Manager. It enables:
- **Communication**: Interacts with the SAP BTP Service Manager via the Open Service Broker API.
- **Provisioning**: Provisions SAP BTP service instances for Kubernetes applications.
- **Credentials**: Retrieves access credentials for applications to use these services.

The operator uses Custom Resource Definitions (CRDs) to manage SAP BTP services through Kubernetes YAML manifests, making service management intuitive and native to Kubernetes.

## Prerequisites

Before installing the operator, ensure you have:
- An SAP BTP global account with a subaccount for service consumption.
- Basic understanding of the SAP BTP Service Manager.
- A Kubernetes cluster (version 1.17 or higher).
- `kubectl` (version 1.17 or higher).
- Helm (version 3.0 or higher).
- Java Runtime Environment (for DITA-OT processing, if applicable).

## Setup

To install the SAP BTP Service Operator in your Kubernetes cluster, follow these steps:

1. **Install cert-manager**:
   - For operator releases v0.1.18 or higher, use cert-manager v1.6.0 or higher.
   - For operator releases v0.1.17 or lower, use cert-manager lower than v1.6.0.

2. **Obtain access credentials**:
   - Create an instance of the SAP Service Manager service (technical name: `service-manager`) with the plan `service-operator-access`.
   - If the plan is not visible, entitle your subaccount for the SAP Service Manager. Refer to [SAP BTP documentation](https://help.sap.com/docs/btp).
   - Create a binding to the service instance and retrieve the generated credentials.

   **Example of default binding object**:
   ```json
   {
       "clientid": "xxxxxxx",
       "clientsecret": "xxxxxxx",
       "url": "https://mysubaccount.authentication.eu10.hana.ondemand.com",
       "xsappname": "b15166|service-manager!b1234",
       "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
   }
   ```

   **Example of binding object with X.509 certificate**:
   ```json
   {
       "clientid": "xxxxxxx",
       "certificate": "-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n",
       "key": "-----BEGIN RSA PRIVATE KEY-----...-----END RSA PRIVATE KEY-----\n",
       "certurl": "https://mysubaccount.authentication.cert.eu10.hana.ondemand.com",
       "xsappname": "b15166|service-manager!b1234",
       "sm_url": "https://service-manager.cfapps.eu10.hana.ondemand.com"
   }
   ```

3. **Add the Helm chart repository**:
   ```bash
   helm repo add sap-btp-operator https://sap.github.io/sap-btp-service-operator
   ```

4. **Deploy the operator**:
   - For clusters using Service Catalog (svcat) and Service Manager agent (for migration), include `--set cluster.id=<clusterID>`.
   - **Example deployment with default credentials**:
     ```bash
     helm upgrade --install sap-btp-operator sap-btp-operator/sap-btp-operator \
         --create-namespace \
         --namespace=sap-btp-operator \
         --set manager.secret.clientid=<clientid> \
         --set manager.secret.clientsecret=<clientsecret> \
         --set manager.secret.sm_url=<sm_url> \
         --set manager.secret.tokenurl=<auth_url>
     ```
   - **Example deployment with X.509 certificate**:
     ```bash
     helm upgrade --install sap-btp-operator sap-btp-operator/sap-btp-operator \
         --create-namespace \
         --namespace=sap-btp-operator \
         --set manager.secret.clientid=<clientid> \
         --set manager.secret.tls.crt="$(cat /path/to/cert)" \
         --set manager.secret.tls.key="$(cat /path/to/key)" \
         --set manager.secret.sm_url=<sm_url> \
         --set manager.secret.tokenurl=<auth_url>
     ```

5. **Verify the access secret**:
   - The credentials are stored in a Kubernetes Secret named `sap-btp-service-operator` in the `sap-btp-operator` namespace.
   - Fields include `clientid`, `clientsecret`, `sm_url`, `tokenurl`, `tokenurlsuffix`, `tls.crt`, and `tls.key`.
   - To rotate credentials, create a new binding, update the Helm deployment with new credentials, and delete the old binding.

## Managing Access

By default, the operator has cluster-wide permissions. To restrict access to specific namespaces, use these Helm parameters:
```bash
helm upgrade --install sap-btp-operator sap-btp-operator/sap-btp-operator \
    --set manager.allow_cluster_access=false \
    --set manager.allowed_namespaces={namespace1,namespace2,...}
```

> **Note**: If `allow_cluster_access` is `true`, `allowed_namespaces` is ignored.

## Working with Multiple Subaccounts

The operator supports connecting a Kubernetes cluster to multiple SAP BTP subaccounts using:
- **Namespace-based mapping**: Link namespaces to different subaccounts via dedicated credentials.
- **Instance-level mapping**: Specify the subaccount for each `ServiceInstance` resource.

Credentials are stored in Secrets in a centrally managed namespace (default: `sap-btp-operator`).

### Subaccount for a Namespace

To associate a namespace with a subaccount, create a Secret named `<namespace-name>-sap-btp-service-operator` in the centrally managed namespace.

**Example default credentials Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-namespace-sap-btp-service-operator
  namespace: sap-btp-operator
type: Opaque
stringData:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

**Example mTLS credentials Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-namespace-sap-btp-service-operator
  namespace: sap-btp-operator
type: Opaque
stringData:
  clientid: "<clientid>"
  tls.crt: "<certificate>"
  tls.key: "<key>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

### Subaccount for a ServiceInstance Resource

To use different subaccounts within the same namespace:
1. Store credentials in a Secret in the centrally managed namespace.
2. Reference the Secret in the `ServiceInstance` resource using `btpAccessCredentialsSecret`.

**Example Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mybtpsecret
  namespace: sap-btp-operator
type: Opaque
stringData:
  clientid: "<clientid>"
  clientsecret: "<clientsecret>"
  sm_url: "<sm_url>"
  tokenurl: "<auth_url>"
  tokenurlsuffix: "/oauth/token"
```

**Example ServiceInstance**:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceInstance
metadata:
  name: sample-instance
spec:
  serviceOfferingName: service-manager
  servicePlanName: subaccount-audit
  btpAccessCredentialsSecret: mybtpsecret
```

### Secrets Precedence

The operator uses credentials in this order:
1. Secret specified in `ServiceInstance` (`btpAccessCredentialsSecret`).
2. Namespace-specific Secret (`<namespace-name>-sap-btp-service-operator`).
3. Default cluster Secret (`sap-btp-service-operator`).

## Using the SAP BTP Service Operator

### Service Instance

To provision an SAP BTP service, create a `ServiceInstance` resource:
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

Apply it:
```bash
kubectl apply -f my-service-instance.yaml
```

Check status:
```bash
kubectl get serviceinstances
```

**Example output**:
```
NAME                  OFFERING        PLAN        STATUS    AGE
my-service-instance   sample-service  sample-plan Created   44s
```

### Service Binding

To access credentials, create a `ServiceBinding` resource:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceBinding
metadata:
  name: sample-binding
spec:
  serviceInstanceName: sample-instance
  externalName: my-binding-external
  secretName: my-secret
  parameters:
    key1: val1
    key2: val2
```

Apply it:
```bash
kubectl apply -f my-binding.yaml
```

Verify:
```bash
kubectl get servicebindings
```

**Example output**:
```
NAME           INSTANCE            STATUS    AGE
sample-binding sample-instance     Created   16s
```

Check the Secret:
```bash
kubectl get secrets
```

**Example output**:
```
NAME        TYPE     DATA   AGE
my-secret   Opaque   5      32s
```

#### Formats of Service Binding Secrets

Secrets store credentials and `ServiceInstance` attributes in various formats.

##### Key-Value Pairs (Default)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sample-binding
stringData:
  uri: https://my-service.authentication.eu10.hana.ondemand.com
  client_id: admin
  client_secret: ********
  instance_guid: your-sample-instance-guid
  instance_name: sample-instance
  plan: sample-plan
  type: sample-service
```

##### Credentials as JSON Object

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sample-binding
stringData:
  myCredentials: '{"uri":"https://my-service.authentication.eu10.hana.ondemand.com","client_id":"admin","client_secret":"********"}'
  instance_guid: your-sample-instance-guid
  instance_name: sample-binding
  plan: sample-plan
  type: sample-service
```

##### Credentials and Service Info as One JSON Object

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sample-binding
stringData:
  myCredentialsAndInstance: '{"uri":"https://my-service.authentication.eu10.hana.ondemand.com","client_id":"admin","client_secret":"********","instance_guid":"your-sample-instance-guid","instance_name":"sample-instance","plan":"sample-plan","type":"sample-service"}'
```

##### Custom Formats

Use `secretTemplate` for custom Secret formats:
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

#### Service Binding Rotation

To enhance security, rotate credentials automatically:
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

After rotation, the Secret updates, and old credentials are stored temporarily in a Secret named `original-secret-name<variable>-guid` until `rotatedBindingTTL` expires.

### Passing Parameters

Use `parameters` or `parametersFrom` in `ServiceInstance` or `ServiceBinding`:
```yaml
apiVersion: services.cloud.sap.com/v1
kind: ServiceInstance
metadata:
  name: my-service-instance
spec:
  parameters:
    name: value
  parametersFrom:
    - secretKeyRef:
        name: my-secret
        key: secret-parameter
```

**Example Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
type: Opaque
stringData:
  secret-parameter: '{"password":"password"}'
```

## Reference Documentation

### Service Instance Properties

**Spec**:
- `serviceOfferingName*`: Name of the SAP BTP service.
- `servicePlanName*`: Plan for the service instance.
- `externalName`: Name in SAP BTP (defaults to `metadata.name`).
- `parameters`: Additional configuration.
- `btpAccessCredentialsSecret`: Secret for SAP BTP credentials.

**Status**:
- `instanceID`: Service instance ID in SAP Service Manager.
- `conditions`: Status conditions (Ready, Failed, Succeeded, Shared).

### Service Binding Properties

**Spec**:
- `serviceInstanceName*`: Name of the ServiceInstance.
- `secretName`: Secret for credentials (defaults to `metadata.name`).
- `secretKey`: Key for credentials as JSON.
- `secretRootKey`: Key for credentials and instance info as JSON.

## Uninstalling the Operator

Delete all service instances and bindings before uninstalling:
```bash
helm uninstall sap-btp-operator -n sap-btp-operator
```

## Troubleshooting and Support


This section provides solutions to common issues encountered when using the SAP BTP Service Operator.

### Cannot Create a Binding Because Service Instance Is in Delete Failed State

**Issue**: You cannot create a binding because the service instance is stuck in a "Delete Failed" state.

**Solution**:
Use the SAP BTP CLI with the `force_k8s_binding=true` parameter to resolve the issue, then delete the binding.

**Command to create a binding**:
```bash
btp create services/binding \
    --subaccount <subaccount-id> \
    --binding <binding-name> \
    --instance-name <instance-name> \
    --service-instance <service-instance-id> \
    --parameters '{"force_k8s_binding": true}' \
    --labels '{"created_by": "user"}'
```

**Command to delete the binding**:
```bash
btp delete services/binding \
    --subaccount <subaccount-id> \
    --binding <binding-name> \
    --instance-name <instance-name> \
    --service-instance <service-instance-id> \
    --parameters '{"force_k8s_binding": false}' \
    --labels '{"created_by": "user"}'
```

> **Note**: Do not use credentials from the `service-operator-access` plan for this operation. This applies only to Kubernetes-managed instances in a "Delete Failed" state. Replace `<subaccount-id>`, `<binding-name>`, `<instance-name>`, and `<service-instance-id>` with your specific values.

### Cluster Is Unavailable, and I Still Have Service Instances and Bindings

**Issue**: You cannot delete service instances or bindings because the Kubernetes cluster is unavailable.

**Solution**:
Use the SAP BTP Service Manager API to clean up resources. This requires credentials from the `subaccount-admin` plan, not the `service-operator-access` plan.

**API Request**:
```
DELETE /v1/platforms/{platformID}/clusters/{clusterID}
```

**Parameters**:
- `platformID`: ID of the platform (obtained from the `service-operator-access` instance ID).
- `clusterID`: Cluster ID from the Setup step 4, or retrieved via the GET service instance/binding API.

**Response Codes**:
- `202`: Request accepted for processing.
- `404`: Platform or cluster not found.
- `429`: Too many requests (rate limit exceeded).

**Headers**:
- `Location`: URL to check operation status (see [SAP BTP Service Manager Operation API](https://help.sap.com/docs/btp)).
- `Retry-After`: Time (in seconds) to wait before retrying (for 429 responses).

> **Warning**: Use this API only for cleaning up unavailable clusters to avoid affecting active clusters. Refer to the [SAP BTP documentation](https://help.sap.com/docs/btp) for details.

### Restoring a Missing Custom Resource

**Issue**: A service instance or binding exists in SAP BTP but is missing as a custom resource (CR) in the Kubernetes cluster.

**Background**:
A missing custom resource can be restored by recreating it with the same name, namespace, and cluster ID, reconnecting it to the existing SAP BTP resource without provisioning a new one.

**Steps**:
1. **Retrieve CR details**:
   - Access the service instance or binding in the SAP BTP Cockpit or via the BTP CLI.
   - Note the custom resource name, namespace, and cluster ID.
2. **Recreate the custom resource**:
   - Use the original YAML manifest, ensuring the same name and namespace.
   - Example:
     ```yaml
     apiVersion: services.cloud.sap.com/v1
     kind: ServiceInstance
     metadata:
       name: my-service-instance
       namespace: my-namespace
     spec:
       serviceOfferingName: sample-service
       servicePlanName: sample-plan
     ```
   - Apply the manifest:
     ```bash
     kubectl apply -f my-service-instance.yaml
     ```
3. **Verify the restoration**:
   - Check if the custom resource is created:
     ```bash
     kubectl get serviceinstances my-service-instance -n my-namespace
     ```
   - Confirm that SAP BTP recognizes the connection.
   - Verify the cluster ID matches (check via SAP BTP Cockpit or BTP CLI). If mismatched, reconfigure the cluster ID in the Helm deployment.

**Support**:
For additional help, raise issues or feature requests on the [GitHub Issues page](https://github.com/sap/sap-btp-service-operator).
<img width="468" height="650" alt="image" src="https://github.com/user-attachments/assets/511e55b6-a6bd-4b81-b168-2dde804d5798" />

