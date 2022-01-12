#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname $0)/.."
tmp_dir=$(mktemp -d)
trap '{ rm -rf -- "$tmp_dir"; }' EXIT

NAMESPACE=kyma-system

cat << EOF > $tmp_dir/req.cnf
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]
organizationName = kyma
commonName       = kyma

[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = sap-btp-operator-webhook-service.$NAMESPACE.svc.cluster.local
DNS.2 = sap-btp-operator-webhook-service.$NAMESPACE.svc.cluster
DNS.3 = sap-btp-operator-webhook-service.$NAMESPACE.svc
DNS.4 = sap-btp-operator-webhook-service.$NAMESPACE
DNS.5 = sap-btp-operator-webhook-service
EOF

CN=sap-btp-operator-webhook-service.$NAMESPACE.svc

# gen root ca
openssl genrsa -out $tmp_dir/ca.key 2048
openssl req -x509 -new -nodes -key $tmp_dir/ca.key -days 3650 -out $tmp_dir/ca.crt -subj "/CN=$CN"

# gen signed certs
openssl genrsa -out $tmp_dir/webhook.key 2048
openssl req -new -key $tmp_dir/webhook.key -out $tmp_dir/csr.pem -subj "/CN=$CN" -config $tmp_dir/req.cnf
openssl x509 -req -in $tmp_dir/csr.pem -CA $tmp_dir/ca.crt -CAkey $tmp_dir/ca.key -CAcreateserial -out $tmp_dir/webhook.crt -days 3650 -extensions v3_req -extfile $tmp_dir/req.cnf

crt=$(cat $tmp_dir/webhook.crt | base64 --wrap=0)
key=$(cat $tmp_dir/webhook.key | base64 --wrap=0)
caBundle=$(cat $tmp_dir/ca.crt | base64 --wrap=0)

sed -i 's/\(.*tls.crt: "\).*\(" # selfsigned\)/\1'"$crt"'\2/' ./chart/templates/secret-certificate.yml
sed -i 's/\(.*tls.key: "\).*\(" # selfsigned\)/\1'"$key"'\2/' ./chart/templates/secret-certificate.yml
sed -i 's/\(.*caBundle: "\).*\(" # selfsigned\)/\1'"$caBundle"'\2/' ./chart/templates/webhook-mutating.yml
sed -i 's/\(.*caBundle: "\).*\(" # selfsigned\)/\1'"$caBundle"'\2/' ./chart/templates/webhook-validating.yml
