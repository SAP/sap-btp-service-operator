#!/usr/bin/env bash
##
## Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved.
##
## Base script created by SAP Oliver Götz.
## This file is part of ewm-cloud-robotics (see https://github.com/SAP/ewm-cloud-robotics).
## Script was enhanced by SAP Steffen Brunner
##
## This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file (https://github.com/SAP/ewm-cloud-robotics/blob/master/LICENSE)

######## ATTENTION ######
## $FOLDER_NAME  /tmp/.gopath/src/$FOLDER_NAME MUST 
## match go.mod "package" definition!
set -e

# K8S branch for code-generator and apimachinery
K8S_BRANCH="release-1.22"
GIT_APIMACHINERY="https://github.com/kubernetes/apimachinery.git"
GIT_CODEGENERATOR="https://github.com/kubernetes/code-generator.git"

# Versions of api to be generated
K8S_GROUPS_VERSIONS="services.cloud.sap.com:v1alpha1"

# Start
echo "### Using kubernetes code-generator and apimachinery from branch: $K8S_BRANCH ###"

# Directory of this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# Prepare temporary environment for generating files
echo "### Prepare temporary environment for generating files ###"
rm -Rf "$SCRIPT_DIR/../tmp/.gopath/src/github.com"

mkdir -p "$SCRIPT_DIR/../tmp/.gopath/src/github.com/SAP/sap-btp-service-operator/api"
mkdir -p "$SCRIPT_DIR/../tmp/.gopath/src/k8s.io"
mkdir -p "$SCRIPT_DIR/../tmp/.gopath/bin"

# Copy preserving file attributes
cp -R -p "$SCRIPT_DIR/api" "$SCRIPT_DIR/../tmp/.gopath/src/github.com/SAP/sap-btp-service-operator"
cd "$SCRIPT_DIR/../tmp/.gopath/src/github.com/SAP/sap-btp-service-operator"

TEMP_REPO=$(pwd)

# Prepare header for generated files
cat > "$TEMP_REPO/HEADER" <<EOF
// Your header goes here...
//
EOF

# Set GOPATH
cd "$SCRIPT_DIR/../tmp/.gopath"
export GOPATH=$(pwd)
export GOBIN=
export GO111MODULE=auto

# Get kubernetes code-generator and apimachinery
echo "### Get kubernetes code-generator and apimachinery ###"
cd "$SCRIPT_DIR/../tmp/.gopath/src/k8s.io"
git clone --branch $K8S_BRANCH $GIT_APIMACHINERY
git clone --branch $K8S_BRANCH $GIT_CODEGENERATOR

# Generate kubernetes client
echo "### Generate kubernetes client ###"
cd "$SCRIPT_DIR/../tmp/.gopath/src"
./k8s.io/code-generator/generate-groups.sh all \
    "github.com/SAP/sap-btp-service-operator/client" \
    "github.com/SAP/sap-btp-service-operator/api" \
    "$K8S_GROUPS_VERSIONS" \
    --go-header-file "$TEMP_REPO/HEADER"

# Copy generated files
echo "### Copy generated files ###"
echo "### Copy new files ###"
# We overwrite presevering attributes here but do not delete old files
#cp -Rf -p "$SCRIPT_DIR/../tmp/.gopath/src/github.com/SAP/sap-btp-service-operator/api" "$SCRIPT_DIR/../client/api"
cp -Rf "$SCRIPT_DIR/../tmp/.gopath/src/github.com/SAP/sap-btp-service-operator/client/" "$SCRIPT_DIR/client/"

# Cleanup temporary environment
echo "### Cleanup temporary environment ###"
# Files in GOPATH/pkg/mod are usually read only. Give write rights that deletion does not fail
chmod -R +w "$SCRIPT_DIR/../tmp/.gopath/pkg"
sleep 1
rm -Rf "$SCRIPT_DIR/../tmp"
