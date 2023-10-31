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

# This script includes common functions for testing setup and teardown.
source $(dirname $0)/../vendor/knative.dev/hack/e2e-tests.sh

# This is HOME directory of this repository for prow.
readonly PLUGIN_DIR="$(dirname "${BASH_SOURCE[0]}")/.."

# This is HOME directory of Knative, where to download the Serving repository.
readonly KNATIVE_DIR=$(dirname ${PLUGIN_DIR})

# This is the branch name of Knative Serving repository, where we run the integration tests.
# The value can be either a specific release branch, e.g. release-1.12 or ${PULL_BASE_REF}.
readonly KNATIVE_REPO_BRANCH="${PULL_BASE_REF}"

export INGRESS_CLASS=${INGRESS_CLASS:-istio.ingress.networking.knative.dev}

# Check if we should use --resolvabledomain.  In case the ingress only has
# hostname, we doesn't yet have a way to support resolvable domain in tests.
function use_resolvable_domain() {
  # Temporarily turning off sslip.io tests, as DNS errors aren't always retried.
  echo "false"
}

function knative_setup() {
  # We will use Istio as the ingress
  install_istio || fail_test "Istio installation failed"

  download_knative "knative/serving" "serving" "${KNATIVE_REPO_BRANCH}"
  echo "Install Knative Serving"
  cd ${KNATIVE_DIR}/"serving"
  ko apply -Rf config/core/
  ko apply -Rf config/hpa-autoscaling
  wait_until_pods_running knative-serving || fail_test "Knative Serving did not come up"

  download_knative "knative-extensions/net-istio" "net-istio" "${KNATIVE_REPO_BRANCH}"
  echo "Install net-istio"
  cd ${KNATIVE_DIR}/"net-istio"
  ko apply -f config/
  wait_until_pods_running knative-serving || fail_test "Knative net-istio did not come up"

  # Replace the controller and autoscaler with the ones in the plugin
  cd ${PLUGIN_DIR}
  kubectl delete deploy autoscaler -n knative-serving
  kubectl delete deploy controller -n knative-serving
  ko apply -Rf config/core/deployments
  wait_until_pods_running knative-serving || fail_test "Knative Serving did not come up for the plugin"
}

# Install Istio.
function install_istio() {
  echo ">> Installing Istio"
  curl -sL https://istio.io/downloadIstioctl | sh -
  $HOME/.istioctl/bin/istioctl install -y
}

# Download the repository of Knative. The purpose of this function is to download the source code of
# knative component for further use, based on component name and branch name.
# Parameters:
#  $1 - component repo name, e.g. knative/serving, knative/eventing, etc.
#  $2 - component name,
#  $3 - branch of the repository.
function download_knative() {
  local component_repo component_name
  component_repo=$1
  component_name=$2
  echo "Download the source code of ${component_name}"
  # Go the directory to download the source code of knative
  cd ${KNATIVE_DIR}
  # Download the source code of knative
  git clone "https://github.com/${component_repo}.git" "${component_name}"
  cd "${component_name}"
  local branch=$3
  if [ -n "${branch}" ] ; then
    git fetch origin ${branch}:${branch}
    git checkout ${branch}
  fi
  cd ${PLUGIN_DIR}
}
