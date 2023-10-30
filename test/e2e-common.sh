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

function knative_setup() {
  # We will use Istio as the ingress
  install_istio || fail_test "Istio installation failed"
  download_knative "knative-extensions/net-istio" "net-istio" "${KNATIVE_REPO_BRANCH}"
  echo "Install net-istio"
  cd ${KNATIVE_DIR}/"net-istio"
  ls

  download_knative "knative/serving" "serving" "${KNATIVE_REPO_BRANCH}"
  echo "Install Knative Serving"
  cd ${KNATIVE_DIR}/"serving"
  ls
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
