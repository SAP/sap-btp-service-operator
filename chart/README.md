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
