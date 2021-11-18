# Local Setup
### Prerequisites
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)

### Deploy locally
Edit [manager secret](../hack/override_values.yaml) section with SM credentials. (DO NOT SUBMIT)
```
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.6.0/cert-manager.yaml
make docker-build
kind load docker-image controller:latest
make deploy
```
### Run tests
`make test`
</br></br>

### Read Logs
```
podName=$(kubectl get pods -A | grep -o "sap-btp-operator-controller-manager-\w*-\w*"); kubectl logs $podName -n sap-btp-operator -c manager
```
