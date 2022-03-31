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
Add annotation to the deployment:
```
sidecar.istio.io/inject: "false"
```

### Move secrets into webhook.yml and define certificates:
```yaml
{{- $cn := printf "sap-btp-operator-webhook-service"  }}
{{- $ca := genCA (printf "%s-%s" $cn "ca") 3650 }}
{{- $altName1 := printf "%s.%s" $cn .Release.Namespace }}
{{- $altName2 := printf "%s.%s.svc" $cn .Release.Namespace }}
{{- $cert := genSignedCert $cn nil (list $altName1 $altName2) 3650 $ca }}
{{- if not .Values.manager.certificates }}
apiVersion: v1
kind: Secret
metadata:
  name: webhook-server-cert
  namespace: {{.Release.Namespace}}
type: kubernetes.io/tls
data:
  tls.crt: {{ b64enc $cert.Cert }}
  tls.key: {{ b64enc $cert.Key }}
---
apiVersion: v1
kind: Secret
metadata:
  name: sap-btp-service-operator-tls
  namespace: {{ .Release.Namespace }}
type: kubernetes.io/tls
data:
  tls.crt: {{ b64enc $cert.Cert }}
  tls.key: {{ b64enc $cert.Key }}
---
{{- end}}
```
Add `caBundle` definition in both webhooks:
```
{{- if not .Values.manager.certificates }}
caBundle: {{ b64enc $ca.Cert }}
{{- end }}
```

### Add sap-btp-operator labels

The deployment and service must contain btp operator specific labels (deployment spec, deployment template and the service label selector):
```yaml
app.kubernetes.io/instance: sap-btp-operator
app.kubernetes.io/name: sap-btp-operator
```
Deployment spec, deployment:
```
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
      app.kubernetes.io/instance: sap-btp-operator
      app.kubernetes.io/name: sap-btp-operator
```

Deployment - template:


# How to publish a new version of a chart
## Download the original chart from helm repository
Configure helm repository:
```
helm repo add sap-btp-operator https://sap.github.io/sap-btp-service-operator
```
Pull the chart
```
helm pull sap-btp-operator/sap-btp-operator
```
Yopu can specify the version if needed:
```
helm pull sap-btp-operator/sap-btp-operator --version v0.2.0
```

Unpack the downloaded tar and apply necessary changes.

## Create a package
```
helm package chart 
```
## Github release
Creawte a github release and upload the generated helm chart (tgz).