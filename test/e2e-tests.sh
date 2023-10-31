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
initialize $@ --skip-istio-addon

# Run the tests
header "Running tests"

failed=0

# Run tests serially in the mesh and https scenarios.
E2E_TEST_FLAGS="${TEST_OPTIONS}"

if [ -z "${E2E_TEST_FLAGS}" ]; then
  E2E_TEST_FLAGS="-resolvabledomain=$(use_resolvable_domain) -ingress-class=${INGRESS_CLASS}"

  # Drop testing alpha and beta features with the Gateway API
  if [[ "${INGRESS_CLASS}" != *"gateway-api"* ]]; then
    E2E_TEST_FLAGS+=" -enable-alpha -enable-beta"
  fi
fi

cd ${KNATIVE_DIR}/"serving"
go_test_e2e -timeout=30m \
  ./test/e2e \
  ${E2E_TEST_FLAGS} || failed=1

(( failed )) && fail_test
success
