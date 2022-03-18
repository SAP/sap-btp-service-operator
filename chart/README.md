# sap-btp-operator Helm Chart

This is a custom version of the sap-btp-operator helm chart.

The upstream version of the sap-btp-operator helm chart has a dependency on the jetstack cert-manager. This custom version makes [jetstack/cert-manager](https://github.com/jetstack/cert-manager) optional and adds the possibility to use a custom caBundle or [gardener/cert-management](https://github.com/gardener/cert-management).

## Prerequeisites

* Kubernetes 1.16+
* Helm 3+

## Install Chart

### With fixed caBundle:
helm install sap-btp-operator . \
    --atomic \
    --create-namespace \
    --namespace=sap-btp-operator \
    --set manager.secret.clientid="<fill in>" \
    --set manager.secret.clientsecret="<fill in>" \
    --set manager.secret.url="<fill in>" \
    --set manager.secret.tokenurl="<fill in>" \
    --set cluster.id="<fill in>"

### With custom caBundle:
helm install sap-btp-operator . \
    --atomic \
    --create-namespace \
    --namespace=sap-btp-operator \
    --set manager.secret.clientid="<fill in>" \
    --set manager.secret.clientsecret="<fill in>" \
    --set manager.secret.url="<fill in>" \
    --set manager.secret.tokenurl="<fill in>" \
    --set manager.certificates.selfSigned.caBundle="${CABUNDLE}" \
    --set manager.certificates.selfSigned.crt="${SERVERCRT}" \
    --set manager.certificates.selfSigned.key="${SERVERKEY}" \
    --set cluster.id="<fill in>"

### With jetstack/cert-manager
helm install sap-btp-operator . \
    --atomic \
    --create-namespace \
    --namespace=sap-btp-operator \
    --set manager.secret.clientid="<fill in>" \
    --set manager.secret.clientsecret="<fill in>" \
    --set manager.secret.url="<fill in>" \
    --set manager.secret.tokenurl="<fill in>" \
    --set manager.certificates.certManager=true \
    --set cluster.id="<fill in>"

### With gardener/cert-management
helm template sap-btp-operator . \
    --atomic \
    --create-namespace \
    --namespace=sap-btp-operator \
    --set manager.secret.clientid="<fill in>" \
    --set manager.secret.clientsecret="<fill in>" \
    --set manager.secret.url="<fill in>" \
    --set manager.secret.tokenurl="<fill in>" \
    --set manager.certificates.certManagement.caBundle="${CABUNDLE}" \
    --set manager.certificates.certManagement.crt=${CACRT} \
    --set manager.certificates.certManagement.key=${CAKEY} \
    --set cluster.id="<fill in>"

# Changes between the chart and the original one
### Istio disabled
Added annotation to the deployment:
```
sidecar.istio.io/inject: "false"
```
### Configure webhook certs
Define hardcoded certs in the webhook configiuration:
configure webhook certificates by adding
```
      {{- if not .Values.manager.certificates }}
      caBundle: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURWekNDQWorZ0F3SUJBZ0lVS1BtaFBlZjlXeGJiR1FyRnU5ZWpWcGFoL2prd0RRWUpLb1pJaHZjTkFRRUwKQlFBd096RTVNRGNHQTFVRUF3d3djMkZ3TFdKMGNDMXZjR1Z5WVhSdmNpMTNaV0pvYjI5ckxYTmxjblpwWTJVdQphM2x0WVMxemVYTjBaVzB1YzNaak1CNFhEVEl5TURFeE1qRTFNamN3T0ZvWERUTXlNREV4TURFMU1qY3dPRm93Ck96RTVNRGNHQTFVRUF3d3djMkZ3TFdKMGNDMXZjR1Z5WVhSdmNpMTNaV0pvYjI5ckxYTmxjblpwWTJVdWEzbHQKWVMxemVYTjBaVzB1YzNaak1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBMDR4NQpLMkcxSU5RZFpyemxTSVJPdC9KVHBmc0tqRGg2aEY0ajBVdTJFSDJJTXc2TFp1dDBTaWwxZFlaZlU0YVAxZFMzClhYZzI3VkpQcm4va2pGQW93S2hDUUFnQllYRjNtK205OUFFbXhVaGdySVpTamR5Y3VpZFlqYytnVGFaeXlFSTIKbGJNUDRlcXBNZXVjSDN0RFJhQmZEUzhhc0sxYXFxWkdkajVZU0h3c3piTmRZUnpDQzRKUVFyQUl2b0lWd0ZXMgpkVXVtY0t6aXFNWVFPTmxOUDN1N1VuWkZCNGljZjF4aEdJZTNlVXBaWGlwMGp0VmtZc1M4RitselV2bThWcmlQCkxKbEhRWGpxUm1GeFA2RGFTWDJhbkl4cDBQdFNpazRjekVSMzJRSCtPbnVjZks5NW9Vd0JXOFpiU2RQMDNRek0KOGNLUXd5dURMS0xsOGh4dW1RSURBUUFCbzFNd1VUQWRCZ05WSFE0RUZnUVVXTlUwcjlSbVIzZ1RvUHlDbEJuawpwSlUzRmNvd0h3WURWUjBqQkJnd0ZvQVVXTlUwcjlSbVIzZ1RvUHlDbEJua3BKVTNGY293RHdZRFZSMFRBUUgvCkJBVXdBd0VCL3pBTkJna3Foa2lHOXcwQkFRc0ZBQU9DQVFFQXh0R0hrQ0plRHpmZ2pqT2dVRzF0ODJCVlYySzEKSkVBVFBNbEZEVmk4aCtmbWx6aGU2QkQ4WHpUVDdBb0VTQmloQk92MzB6UVd1a3NFT0NvbmV4V1hDOEcyQVQ5cAphS05BNG1WbDNUelgrTDRHVHNVOEhtcnAvYWE2MlVoRTBySDdvMGZJS2gxZk4wcHIyT0xKZldPYWpSeWRKQ1RuCjYxZW1xVjIwK0xReVdoNTVtYm9COUZRN3JlTjkzdHRNcm9LM3R1c0VSckJCREV0NFVHaWFtTFFuZm4wOS9jOFcKZEt5YXFqclVtVFhMWjNaVWdHUStGeFZSTWIwamRKa2dpZnFFQnBLUENWNzhYTzQrQW5LUm5qa2dIUTZqRmtJNgo3V1B3eDFZTmIvNThrbFM1clZYc3ZsWnQwTUpMYjllanFKRit3Qk9jbXhPS3dPbHdDZEs4VlRKZmhnPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=" # selfsigned
      {{- end }}
``` 
### Add secret-certificate.yml

# Create a package
```
helm package chart 
```