make docker-build IMG=localhost:5000/controller:latest
make docker-push IMG=localhost:5000/controller:latest
./hack/kind-with-registry.sh
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.6.1/cert-manager.yaml
make deploy IMG=localhost:5000/controller