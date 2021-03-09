#!/bin/bash

uuid()
{
    local N B T

    for (( N=0; N < 16; ++N ))
    do
        B=$(( $RANDOM%255 ))

        if (( N == 6 ))
        then
            printf '4%x' $(( B%15 ))
        elif (( N == 8 ))
        then
            local C='89ab'
            printf '%c%x' ${C:$(( $RANDOM%${#C} )):1} $(( B%15 ))
        else
            printf '%02x' $B
        fi

        for T in 3 5 7 9
        do
            if (( T == N ))
            then
                printf '-'
                break
            fi
        done
    done

    echo
}

store_configmap() {
  local management_namespace=$1
  local clusterid=$(uuid)
  kubectl create -f - <<EOT
apiVersion: v1
kind: ConfigMap
data:
  CLUSTER_ID: "$clusterid"
  MANAGEMENT_NAMESPACE: "$management_namespace"
metadata:
  name: sap-btp-operator-config
  namespace: operators
EOT
}

store_secret() {
  local clientid clientsecret url tokenurl
  clientid=$(printf '%s' "$1" | base64)
  clientsecret=$(printf '%s' "$2" | base64)
  url=$(printf '%s' "$3" | base64)
  tokenurl=$(printf '%s' "$4" | base64)
  management_namespace=$5

  kubectl apply -f - <<EOT
apiVersion: v1
kind: Secret
metadata:
  name: sap-btp-service-operator
  namespace: $management_namespace
type: Opaque
data:
  clientid: $clientid
  clientsecret: $clientsecret
  url: $url
  tokenurl: $tokenurl
EOT
}

usage="SAP CP Operator Setup
namespace parameter indicates the namespace where SM secret can be found, default namespace is 'operators'
Usage:
  setup_operator_env <clientid> <clientsecret> <url> <tokenurl> -n <namespace>
"

if [ "$#" -lt 4 ]; then
  echo "$usage"
  exit 1
fi

namespace="operators"
clientid=$1
clientsecret=$2
url=$3
tokenurl=$4
shift
shift
shift
shift

while test $# -gt 0; do
  case "$1" in
    -n)
      shift
      if test $# -gt 0; then
        export namespace=$1
      else
        echo "no namespace specified"
        exit 1
      fi
      shift
      ;;
    *)
    echo "$usage"
    exit 1
  esac
done

kubectl create namespace operators --dry-run=client -o yaml | kubectl apply -f -
store_configmap "$namespace"
store_secret "$clientid" "$clientsecret" "$url" "$tokenurl" "$namespace"
