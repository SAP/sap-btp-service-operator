<p><a href="https://coveralls.io/github/SAP/sap-btp-service-operator?branch=main"><img src="https://coveralls.io/repos/github/SAP/sap-btp-service-operator/badge.svg?branch=main" alt="Coverage Status"></a>
<a href="https://github.com/SAP/sap-btp-service-operator/actions"><img src="https://github.com/SAP/sap-btp-service-operator/workflows/Go/badge.svg" alt="Build Status"></a>
<a href="https://github.com/SAP/sap-btp-service-operator/blob/master/LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License"></a>
<a href="https://goreportcard.com/report/github.com/SAP/sap-btp-service-operator"><img src="https://goreportcard.com/badge/github.com/SAP/sap-btp-service-operator" alt="Go Report Card"></a>
<a href="https://api.reuse.software/info/github.com/SAP/sap-btp-service-operator"><img src="https://api.reuse.software/badge/github.com/SAP/sap-btp-service-operator" alt="REUSE status"></a></p>
<h1 id="sap-business-technology-platform-sap-btp-service-operator-for-kubernetes">SAP Business Technology Platform (SAP BTP) Service Operator for Kubernetes</h1>
<p>With the SAP BTP service operator, you can consume <a href="https://platformx-d8bd51250.dispatcher.us2.hana.ondemand.com/protected/index.html#/viewServices?">SAP BTP services</a> from your Kubernetes cluster using Kubernetes-native tools. 
SAP BTP service operator allows you to provision and manage service instances and service bindings of SAP BTP services so that your Kubernetes-native applications can access and use needed services from the cluster.<br>The SAP BTP service operator is based on the <a href="https://kubernetes.io/docs/concepts/extend-kubernetes/operator/">Kubernetes Operator pattern</a>.</p>
<h2 id="note">Note</h2>
<p>This feature is still under development, review, and testing. </p>
<h2 id="table-of-contents">Table of Contents</h2>
<ul>
<li><a href="#prerequisites">Prerequisites</a></li>
<li><a href="#setup">Setup Operator</a></li>
<li><a href="#sap-btp-kubectl-plugin-experimental">SAP BTP kubectl extension</a></li>
<li><a href="#using-the-sap-btp-service-operator">Using the SAP BTP Service Operator</a><ul>
<li><a href="#step-1-create-a-service-instance">Creating a service instance</a></li>
<li><a href="#step-2-create-a-service-binding">Binding the service instance</a></li>
</ul>
</li>
<li><a href="#reference-documentation">Reference Documentation</a><ul>
<li><a href="#service-instance">Service instance properties</a></li>
<li><a href="#service-binding">Binding properties</a></li>
<li><a href="#passing-parameters">Passing parameters</a></li>
</ul>
</li>
</ul>
<h2 id="prerequisites">Prerequisites</h2>
<ul>
<li>SAP BTP <a href="https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/d61c2819034b48e68145c45c36acba6e.html">Global Account</a> and <a href="https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/55d0b6d8b96846b8ae93b85194df0944.html">Subaccount</a> </li>
<li>Service Management Control (SMCTL) command line interface. See <a href="https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/0107f3f8c1954a4e96802f556fc807e3.html">Using the SMCTL</a>.</li>
<li><a href="https://kubernetes.io/">Kubernetes cluster</a> running version 1.17 or higher </li>
<li><a href="https://kubernetes.io/docs/tasks/tools/install-kubectl/">kubectl</a> v1.17 or higher</li>
<li><a href="https://helm.sh/">helm</a> v3.0 or higher</li>
</ul>
<p><a href="#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes">Back to top</a></p>
<h2 id="setup">Setup</h2>
<ol>
<li><p>Install <a href="https://cert-manager.io/docs/installation/kubernetes">cert-manager</a></p>
<ul>
<li>for releases v0.1.18 or higher use cert manager v1.6.0 or higher </li>
<li>for releases v0.1.17 or lower use cert manager lower then v1.6.0</li>
</ul>
</li>
<li><p>Obtain the access credentials for the SAP BTP service operator:</p>
<p>a. Using the SAP BTP cockpit or CLI, create an instance of the SAP Service Manager service (technical name: <code>service-manager</code>) with the plan:
 <code>service-operator-access</code><br/><br><em>Note</em><br/><br><em>If you can&#39;t see the needed plan, you need to entitle your subaccount to use SAP Service Manager service.</em><br></p>
<p>   <em>For more information about how to entitle a service to a subaccount, see:</em></p>
<ul>
<li><em><a href="https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/Cloud/en-US/5ba357b4fa1e4de4b9fcc4ae771609da.html">Configure Entitlements and Quotas for Subaccounts</a></em></li>
</ul>
</li>
</ol>
<pre><code>  <span class="xml"><span class="hljs-tag">&lt;<span class="hljs-name">br</span>/&gt;</span></span>For more information about creating service instances, see:     
  * [<span class="hljs-string">Creating Service Instances Using the SAP BTP Cockpit</span>](<span class="hljs-link">https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/bf71f6a7b7754dbd9dfc2569791ccc96.html</span>)

  * [<span class="hljs-string">Creating Service Instances using SMCTL</span>](<span class="hljs-link">https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/b327b66b711746b085ec5d2ea16e608e.html</span>)<span class="xml"><span class="hljs-tag">&lt;<span class="hljs-name">br</span>&gt;</span></span> 
</code></pre><p>   b. Create a binding to the created service instance.</p>
<p>   For more information about creating service bindings, see:  </p>
<pre><code>  * [<span class="hljs-string">Creating Service Bindings Using the SAP BTP Cockpit</span>](<span class="hljs-link">https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/55b31ea23c474f6ba2f64ee4848ab1b3.html</span>) 

  * [<span class="hljs-string">Creating Service Bindings Using SMCTL</span>](<span class="hljs-link">https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/f53ff2634e0a46d6bfc72ec075418dcd.html</span>). 
</code></pre><p>   c. Retrieve the generated access credentials from the created binding:</p>
<pre><code>  The example <span class="hljs-keyword">of</span> the default binding object used <span class="hljs-keyword">if</span> no credentials type is specified:

```json
 {
     <span class="hljs-string">"clientid"</span>: <span class="hljs-string">"xxxxxxx"</span>,
     <span class="hljs-string">"clientsecret"</span>: <span class="hljs-string">"xxxxxxx"</span>,
     <span class="hljs-string">"url"</span>: <span class="hljs-string">"https://mysubaccount.authentication.eu10.hana.ondemand.com"</span>,
     <span class="hljs-string">"xsappname"</span>: <span class="hljs-string">"b15166|service-manager!b1234"</span>,
     <span class="hljs-string">"sm_url"</span>: <span class="hljs-string">"https://service-manager.cfapps.eu10.hana.ondemand.com"</span>
 }
```
The example <span class="hljs-keyword">of</span> the binding object <span class="hljs-keyword">with</span> the specified X<span class="hljs-number">.509</span> credentials type:

```json
{
     <span class="hljs-string">"clientid"</span>: <span class="hljs-string">"xxxxxxx"</span>,
     <span class="hljs-string">"certificate"</span>: <span class="hljs-string">"-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----..-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----...-----END CERTIFICATE-----\n"</span>,
     <span class="hljs-string">"key"</span>: <span class="hljs-string">"-----BEGIN RSA PRIVATE KEY-----...-----END RSA PRIVATE KEY-----\n"</span>,
     <span class="hljs-string">"certurl"</span>: <span class="hljs-string">"https://mysubaccount.authentication.cert.eu10.hana.ondemand.com"</span>,
     <span class="hljs-string">"xsappname"</span>: <span class="hljs-string">"b15166|service-manager!b1234"</span>,
     <span class="hljs-string">"sm_url"</span>: <span class="hljs-string">"https://service-manager.cfapps.eu10.hana.ondemand.com"</span>
 }
```
</code></pre><ol>
<li>Add SAP BTP service operator chart repository  <pre><code class="lang-bash"> helm repo <span class="hljs-keyword">add</span> sap-btp-<span class="hljs-keyword">operator</span> https:<span class="hljs-comment">//sap.github.io/sap-btp-service-operator</span>
</code></pre>
</li>
<li><p>Deploy the the SAP BTP service operator in the cluster using the obtained access credentials:<br></p>
<p><em>Note:<br>
 If you are deploying the SAP BTP service operator in the registered cluster based on the Service Catalog (svcat) and Service Manager agent so that you can migrate svcat-based content to service operator-based content, add <code>--set cluster.id=&lt;clusterID&gt;</code> to your deployment script.</em><br><em>For more information, see the step 2 of the Setup section of <a href="https://github.com/SAP/sap-btp-service-operator-migration/blob/main/README.md">Migration to SAP BTP service operator</a>.</em></p>
<p>The example of the deployment that uses the default access credentials type:</p>
<pre><code class="lang-bash"> helm upgrade --install &lt;release-name&gt; sap-btp-operator/sap-btp-operator \
     --create-namespace \
     --namespace=sap-btp-operator \        
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.clientid</span>=&lt;clientid&gt; \
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.clientsecret</span>=&lt;clientsecret&gt; \
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.url</span>=&lt;sm_url&gt; \
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.tokenurl</span>=&lt;url&gt;
</code></pre>
<p>The example of the deployment that uses the X.509 access credentials type:</p>
<pre><code class="lang-bash"> helm upgrade --install &lt;release-name&gt; sap-btp-operator/sap-btp-operator \
     --create-namespace \
     --namespace=sap-btp-operator \        
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.clientid</span>=&lt;clientid&gt; \
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.tls</span><span class="hljs-selector-class">.crt</span>=<span class="hljs-string">"$(cat /path/to/cert)"</span> \
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.tls</span><span class="hljs-selector-class">.key</span>=<span class="hljs-string">"$(cat /path/to/key)"</span> \
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.url</span>=&lt;sm_url&gt; \
     --set manager<span class="hljs-selector-class">.secret</span><span class="hljs-selector-class">.tokenurl</span>=&lt;certurl&gt;
</code></pre>
</li>
</ol>
<p><a href="#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes">Back to top</a>.</p>
<h2 id="using-the-sap-btp-service-operator">Using the SAP BTP Service Operator</h2>
<h4 id="step-1-create-a-service-instance">Step 1: Create a service instance</h4>
<ol>
<li>To create an instance of a service offered by SAP BTP, first create a <code>ServiceInstance</code> custom-resource file:</li>
</ol>
<pre><code class="lang-yaml"><span class="hljs-symbol">    apiVersion:</span> services.cloud.sap.com/v1alpha1
<span class="hljs-symbol">    kind:</span> ServiceInstance
<span class="hljs-symbol">    metadata:</span>
<span class="hljs-symbol">        name:</span> my-service-instance
<span class="hljs-symbol">    spec:</span>
<span class="hljs-symbol">        serviceOfferingName:</span> sample-service
<span class="hljs-symbol">        servicePlanName:</span> sample-plan
<span class="hljs-symbol">        externalName:</span> my-service-instance-external
<span class="hljs-symbol">        parameters:</span>
<span class="hljs-symbol">          key1:</span> val1
<span class="hljs-symbol">          key2:</span> val2
</code></pre>
<ul>
<li><p><code>&lt;offering&gt;</code> - The name of the SAP BTP service that you want to create. 
To learn more about viewing and managing the available services for your subaccount in the SAP BTP cockpit, see <a href="https://help.sap.com/viewer/09cc82baadc542a688176dce601398de/Cloud/en-US/affcc245c332433ba71917ff715b9971.html">Service Marketplace</a>. </p>
<p> Tip: Use the <em>Environment</em> filter to get all offerings that are relevant for Kubernetes.</p>
</li>
<li><code>&lt;plan&gt;</code> - The plan of the selected service offering that you want to create.</li>
</ul>
<ol>
<li><p>Apply the custom-resource file in your cluster to create the instance.</p>
<pre><code class="lang-bash">kubectl apply -f path/<span class="hljs-keyword">to</span>/<span class="hljs-keyword">my</span>-service-instance.yaml
</code></pre>
</li>
<li><p>Check that the status of the service in your cluster is <strong>Created</strong>.</p>
<pre><code class="lang-bash">kubectl <span class="hljs-built_in">get</span> serviceinstances
NAME                  OFFERING          PLAN        STATUS    AGE
my-service-instance   <span class="hljs-symbol">&lt;offering&gt;</span>        <span class="hljs-symbol">&lt;plan&gt;</span>      Created   <span class="hljs-number">44</span>s
</code></pre>
<p><a href="#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes">Back to top</a></p>
</li>
</ol>
<h4 id="step-2-create-a-service-binding">Step 2: Create a Service Binding</h4>
<ol>
<li><p>To get access credentials to your service instance and make it available in the cluster so that your applications can use it, create a <code>ServiceBinding</code> custom resource, and set the <code>serviceInstanceName</code> field to the name of the <code>ServiceInstance</code> resource you created.</p>
<p>The credentials are stored in a secret created in your cluster.</p>
<pre><code class="lang-yaml"><span class="hljs-symbol">apiVersion:</span> services.cloud.sap.com/v1alpha1
<span class="hljs-symbol">kind:</span> ServiceBinding
<span class="hljs-symbol">metadata:</span>
<span class="hljs-symbol">    name:</span> my-binding
<span class="hljs-symbol">spec:</span>
<span class="hljs-symbol">    serviceInstanceName:</span> my-service-instance
<span class="hljs-symbol">    externalName:</span> my-binding-external
<span class="hljs-symbol">    secretName:</span> mySecret
<span class="hljs-symbol">    parameters:</span>
<span class="hljs-symbol">      key1:</span> val1
<span class="hljs-symbol">      key2:</span> val2
</code></pre>
</li>
<li><p>Apply the custom resource file in your cluster to create the binding.</p>
<pre><code class="lang-bash">kubectl apply -f path/<span class="hljs-keyword">to</span>/<span class="hljs-keyword">my</span>-binding.yaml
</code></pre>
</li>
<li><p>Check that your binding status is <strong>Created</strong>.</p>
<pre><code class="lang-bash">kubectl <span class="hljs-keyword">get</span> servicebindings
NAME         INSTANCE              STATUS    AGE
<span class="hljs-keyword">my</span>-binding   <span class="hljs-keyword">my</span>-service-instance   Created   <span class="hljs-number">16</span>s
</code></pre>
</li>
<li><p>Check that a secret with the same name as the name of your binding is created. The secret contains the service credentials that apps in your cluster can use to access the service.</p>
<pre><code class="lang-bash"><span class="hljs-symbol">kubectl</span> <span class="hljs-meta">get</span> secrets
<span class="hljs-symbol">NAME</span>         TYPE     <span class="hljs-meta">DATA</span>   AGE
<span class="hljs-symbol">my</span>-<span class="hljs-keyword">binding </span>  Opaque   <span class="hljs-number">5</span>      <span class="hljs-number">32</span>s
</code></pre>
<p>See <a href="https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets">Using Secrets</a> to learn about different options on how to use the credentials from your application running in the Kubernetes cluster, </p>
</li>
</ol>
<p><a href="#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes">Back to top</a></p>
<h2 id="reference-documentation">Reference Documentation</h2>
<h3 id="service-instance">Service Instance</h3>
<h4 id="spec">Spec</h4>
<table>
<thead>
<tr>
<th style="text-align:left">Parameter</th>
<th style="text-align:left">Type</th>
<th style="text-align:left">Description</th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align:left">serviceOfferingName<code>*</code></td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The name of the SAP BTP service offering.</td>
</tr>
<tr>
<td style="text-align:left">servicePlanName<code>*</code></td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The plan to use for the service instance.</td>
</tr>
<tr>
<td style="text-align:left">servicePlanID</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The plan ID in case service offering and plan name are ambiguous.</td>
</tr>
<tr>
<td style="text-align:left">externalName</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The name for the service instance in SAP BTP, defaults to the instance <code>metadata.name</code> if not specified.</td>
</tr>
<tr>
<td style="text-align:left">parameters</td>
<td style="text-align:left"><code>[]object</code></td>
<td style="text-align:left">Some services support the provisioning of additional configuration parameters during the instance creation.<br/>For the list of supported parameters, check the documentation of the particular service offering.</td>
</tr>
<tr>
<td style="text-align:left">parametersFrom</td>
<td style="text-align:left"><code>[]object</code></td>
<td style="text-align:left">List of sources to populate parameters.</td>
</tr>
<tr>
<td style="text-align:left">customTags</td>
<td style="text-align:left"><code>[]string</code></td>
<td style="text-align:left">List of custom tags describing the ServiceInstance, will be copied to <code>ServiceBinding</code> secret in the key called <code>tags</code>.</td>
</tr>
<tr>
<td style="text-align:left">userInfo</td>
<td style="text-align:left"><code>object</code></td>
<td style="text-align:left">Contains information about the user that last modified this service instance.</td>
</tr>
</tbody>
</table>
<h4 id="status">Status</h4>
<table>
<thead>
<tr>
<th style="text-align:left">Parameter</th>
<th style="text-align:left">Type</th>
<th style="text-align:left">Description</th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align:left">instanceID</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The service instance ID in SAP Service Manager service.</td>
</tr>
<tr>
<td style="text-align:left">operationURL</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The URL of the current operation performed on the service instance.</td>
</tr>
<tr>
<td style="text-align:left">operationType</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The type of the current operation. Possible values are CREATE, UPDATE, or DELETE.</td>
</tr>
<tr>
<td style="text-align:left">conditions</td>
<td style="text-align:left"><code>[]condition</code></td>
<td style="text-align:left">An array of conditions describing the status of the service instance.<br/>The possible condition types are:<br>- <code>Ready</code>: set to <code>true</code>  if the instance is ready and usable<br/>- <code>Failed</code>: set to <code>true</code> when an operation on the service instance fails.<br/> In the case of failure, the details about the error are available in the condition message.<br>- <code>Succeeded</code>: set to <code>true</code> when an operation on the service instance succeeded. In case of <code>false</code> operation considered as in progress unless <code>Failed</code> condition exists.</td>
</tr>
<tr>
<td style="text-align:left">tags</td>
<td style="text-align:left"><code>[]string</code></td>
<td style="text-align:left">Tags describing the ServiceInstance as provided in service catalog, will be copied to <code>ServiceBinding</code> secret in the key called <code>tags</code>.</td>
</tr>
</tbody>
</table>
<h3 id="service-binding">Service Binding</h3>
<h4 id="spec">Spec</h4>
<table>
<thead>
<tr>
<th style="text-align:left">Parameter</th>
<th style="text-align:left">Type</th>
<th style="text-align:left">Description</th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align:left">serviceInstanceName<code>*</code></td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The Kubernetes name of the service instance to bind, should be in the namespace of the binding.</td>
</tr>
<tr>
<td style="text-align:left">externalName</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The name for the service binding in SAP BTP, defaults to the binding <code>metadata.name</code> if not specified.</td>
</tr>
<tr>
<td style="text-align:left">secretName</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The name of the secret where the credentials are stored, defaults to the binding <code>metadata.name</code> if not specified.</td>
</tr>
<tr>
<td style="text-align:left">secretKey</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The key inside the binding secret to store the credentials returned by the broker encoded as json to support complex data structures.</td>
</tr>
<tr>
<td style="text-align:left">secretRootKey</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The key inside the secret to store all binding data including credentials returned by the broker and additional info under single key.<br/>Convenient way to store whole binding data in single file when using <code>volumeMounts</code>.</td>
</tr>
<tr>
<td style="text-align:left">parameters</td>
<td style="text-align:left"><code>[]object</code></td>
<td style="text-align:left">Some services support the provisioning of additional configuration parameters during the bind request.<br/>For the list of supported parameters, check the documentation of the particular service offering.</td>
</tr>
<tr>
<td style="text-align:left">parametersFrom</td>
<td style="text-align:left"><code>[]object</code></td>
<td style="text-align:left">List of sources to populate parameters.</td>
</tr>
<tr>
<td style="text-align:left">userInfo</td>
<td style="text-align:left"><code>object</code></td>
<td style="text-align:left">Contains information about the user that last modified this service binding.</td>
</tr>
</tbody>
</table>
<h4 id="status">Status</h4>
<table>
<thead>
<tr>
<th style="text-align:left">Parameter</th>
<th style="text-align:left">Type</th>
<th style="text-align:left">Description</th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align:left">instanceID</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The ID of the bound instance in the SAP Service Manager service.</td>
</tr>
<tr>
<td style="text-align:left">bindingID</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The service binding ID in SAP Service Manager service.</td>
</tr>
<tr>
<td style="text-align:left">operationURL</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The URL of the current operation performed on the service binding.</td>
</tr>
<tr>
<td style="text-align:left">operationType</td>
<td style="text-align:left"><code>string</code></td>
<td style="text-align:left">The type of the current operation. Possible values are CREATE, UPDATE, or DELETE.</td>
</tr>
<tr>
<td style="text-align:left">conditions</td>
<td style="text-align:left"><code>[]condition</code></td>
<td style="text-align:left">An array of conditions describing the status of the service instance.<br/>The possible conditions types are:<br/>- <code>Ready</code>: set to <code>true</code> if the binding is ready and usable<br/>- <code>Failed</code>: set to <code>true</code> when an operation on the service binding fails.<br/> In the case of failure, the details about the error are available in the condition message.<br>- <code>Succeeded</code>: set to <code>true</code> when an operation on the service binding succeeded. In case of <code>false</code> operation considered as in progress unless <code>Failed</code> condition exists.</td>
</tr>
</tbody>
</table>
<p><a href="#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes">Back to top</a></p>
<h3 id="passing-parameters">Passing Parameters</h3>
<p>To set input parameters, you may use the <code>parameters</code> and <code>parametersFrom</code>
fields in the <code>spec</code> field of the <code>ServiceInstance</code> or <code>ServiceBinding</code> resource:</p>
<ul>
<li><code>parameters</code> : can be used to specify a set of properties to be sent to the
broker. The data specified will be passed &quot;as-is&quot; to the broker without any
modifications - aside from converting it to JSON for transmission to the broker
in the case of the <code>spec</code> field being specified as <code>YAML</code>. Any valid <code>YAML</code> or
<code>JSON</code> constructs are supported. Only one parameters field may be specified per
<code>spec</code>.</li>
<li><code>parametersFrom</code> : can be used to specify which secret, and key in that secret,
which contains a <code>string</code> that represents the json to include in the set of
parameters to be sent to the broker. The <code>parametersFrom</code> field is a list which
supports multiple sources referenced per <code>spec</code>.</li>
</ul>
<p>You may use either, or both, of these fields as needed.</p>
<p>If multiple sources in <code>parameters</code> and <code>parametersFrom</code> blocks are specified,
the final payload is a result of merging all of them at the top level.
If there are any duplicate properties defined at the top level, the specification
is considered to be invalid, the further processing of the <code>ServiceInstance</code>/<code>ServiceBinding</code>
resource stops and its <code>status</code> is marked with error condition.</p>
<p>The format of the <code>spec</code> will be (in YAML format):</p>
<pre><code class="lang-yaml"><span class="hljs-attribute">spec</span>:
  ...
  <span class="hljs-attribute">parameters</span>:
    <span class="hljs-attribute">name</span>: value
  <span class="hljs-attribute">parametersFrom</span>:
    - <span class="hljs-attribute">secretKeyRef</span>:
        <span class="hljs-attribute">name</span>: my-secret
        <span class="hljs-attribute">key</span>: secret-parameter
</code></pre>
<p>or, in JSON format</p>
<pre><code class="lang-json"><span class="hljs-string">"spec"</span>: {
  <span class="hljs-string">"parameters"</span>: {
    <span class="hljs-string">"name"</span>: <span class="hljs-string">"value"</span>
  },
  <span class="hljs-string">"parametersFrom"</span>: {
    <span class="hljs-string">"secretKeyRef"</span>: {
      <span class="hljs-string">"name"</span>: <span class="hljs-string">"my-secret"</span>,
      <span class="hljs-string">"key"</span>: <span class="hljs-string">"secret-parameter"</span>
    }
  }
}
</code></pre>
<p>and the secret would need to have a key named secret-parameter:</p>
<pre><code class="lang-yaml"><span class="hljs-attr">apiVersion:</span> v1
<span class="hljs-attr">kind:</span> Secret
<span class="hljs-attr">metadata:</span>
<span class="hljs-attr">  name:</span> my-secret
<span class="hljs-attr">type:</span> Opaque
<span class="hljs-attr">stringData:</span>
<span class="hljs-attr">  secret-parameter:</span>
    <span class="hljs-string">'{
      "password": "letmein"
    }'</span>
</code></pre>
<p>The final JSON payload to be sent to the broker would then look like:</p>
<pre><code class="lang-json">{
  <span class="hljs-attr">"name"</span>: <span class="hljs-string">"value"</span>,
  <span class="hljs-attr">"password"</span>: <span class="hljs-string">"letmein"</span>
}
</code></pre>
<p>Multiple parameters could be listed in the secret - simply separate key/value pairs with a comma as in this example:</p>
<pre><code class="lang-yaml">  secret-<span class="hljs-keyword">parameter</span>:
    '{
      <span class="hljs-string">"password"</span>: <span class="hljs-string">"letmein"</span>,
      <span class="hljs-string">"key2"</span>: <span class="hljs-string">"value2"</span>,
      <span class="hljs-string">"key3"</span>: <span class="hljs-string">"value3"</span>
    }'
</code></pre>
<h2 id="support">Support</h2>
<p>You&#39;re welcome to raise issues related to feature requests, bugs, or give us general feedback on this project&#39;s GitHub Issues page. 
The SAP BTP service operator project maintainers will respond to the best of their abilities. </p>
<h2 id="contributions">Contributions</h2>
<p>We currently do not accept community contributions. </p>
<h2 id="sap-btp-kubectl-plugin-experimental-">SAP BTP kubectl Plugin (Experimental)</h2>
<p>The SAP BTP kubectl plugin extends kubectl with commands for getting the available services in your SAP BTP account by
using the access credentials stored in the cluster.</p>
<h3 id="prerequisites">Prerequisites</h3>
<ul>
<li><a href="https://stedolan.github.io/jq/">jq</a></li>
</ul>
<h3 id="limitations">Limitations</h3>
<ul>
<li>The SAP BTP kubectl plugin is currently based on <code>bash</code>. If you&#39;re using Windows, you should utilize the SAP BTP plugin commands from a linux shell (e.g. <a href="https://www.cygwin.com/">Cygwin</a>).  </li>
</ul>
<h3 id="installation">Installation</h3>
<ul>
<li>Download <code>https://github.com/SAP/sap-btp-service-operator/releases/download/&lt;release&gt;/kubectl-sapbtp</code></li>
<li>Move the executable file to any location in your <code>PATH</code></li>
</ul>
<p>The list of available releases: <a href="https://github.com/SAP/sap-btp-service-operator/releases">sapbtp-operator releases</a></p>
<h3 id="usage">Usage</h3>
<pre><code>  kubectl sapbtp marketplace -n &lt;<span class="hljs-keyword">namespace</span>&gt;
  kubectl sapbtp plans -n &lt;<span class="hljs-keyword">namespace</span>&gt;
  kubectl sapbtp services -n &lt;<span class="hljs-keyword">namespace</span>&gt;
</code></pre><p>Use the <code>namespace</code> parameter to specify the location of the secret containing the SAP BTP access credentials.<br>Usually it is the namespace in which you installed the operator. 
If not specified, the <code>default</code> namespace is used. </p>
<p><a href="#sap-business-technology-platform-sap-btp-service-operator-for-kubernetes">Back to top</a></p>
<h2 id="license">License</h2>
<p>This project is licensed under Apache 2.0 except as noted otherwise in the <a href="./LICENSE">license</a> file.</p>
