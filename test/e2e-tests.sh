#!/bin/bash

# Copyright 2023 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

source $(dirname $0)/e2e-common.sh

# Script entry point.
initialize --num-nodes=4 --enable-ha --cluster-version=1.26 "$@"

# Run the tests
header "Running tests"

failed=0

# Run tests serially in the mesh and https scenarios.
GO_TEST_FLAGS=""
E2E_TEST_FLAGS="${TEST_OPTIONS}"

if [ -z "${E2E_TEST_FLAGS}" ]; then
  E2E_TEST_FLAGS="-resolvabledomain=$(use_resolvable_domain) -ingress-class=${INGRESS_CLASS}"

  # Drop testing alpha and beta features with the Gateway API
  if [[ "${INGRESS_CLASS}" != *"gateway-api"* ]]; then
    E2E_TEST_FLAGS+=" -enable-alpha -enable-beta"
  fi
fi

if (( HTTPS )); then
  E2E_TEST_FLAGS+=" -https"
  toggle_feature external-domain-tls Enabled config-network
  kubectl apply -f "${E2E_YAML_DIR}"/test/config/externaldomaintls/certmanager/caissuer/
  add_trap "kubectl delete -f ${E2E_YAML_DIR}/test/config/externaldomaintls/certmanager/caissuer/ --ignore-not-found" SIGKILL SIGTERM SIGQUIT
fi

if (( MESH )); then
  GO_TEST_FLAGS+=" -parallel 1"
fi

if (( SHORT )); then
  GO_TEST_FLAGS+=" -short"
fi

cd ${KNATIVE_DIR}/"serving"
go_test_e2e -timeout=30m \
  ${GO_TEST_FLAGS} \
  ./test/e2e \
  ${E2E_TEST_FLAGS} || failed=1

(( failed )) && fail_test
success
