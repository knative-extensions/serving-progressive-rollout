# Knative Serving Progressive Rollout

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/knative-extensions/serving-progressive-rollout)
[![Go Report Card](https://goreportcard.com/badge/knative-extensions/serving-progressive-rollout)](https://goreportcard.com/report/knative-extensions/serving-progressive-rollout)
[![Releases](https://img.shields.io/github/release-pre/knative-extensions/serving-progressive-rollout.svg?sort=semver)](https://github.com/knative-extensions/serving-progressive-rollout/releases)
[![LICENSE](https://img.shields.io/github/license/knative-extensions/serving-progressive-rollout.svg)](https://github.com/knative-extensions/serving-progressive-rollout/blob/main/LICENSE)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://cloud-native.slack.com/archives/C04LGHDR9K7)
[![codecov](https://codecov.io/gh/knative-extensions/serving-progressive-rollout/branch/main/graph/badge.svg)](https://app.codecov.io/gh/knative-extensions/serving-progressive-rollout)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/5913/badge)](https://bestpractices.coreinfrastructure.org/projects/5913)

Knative Serving Progressive Rollout is an extension to the Knative Serving. It provides middleware primitives that enable:

- The progressive rollout of a new version based on an existing version

The main purpose of this project is to optimize the usage of the resources during the transitional time for the Knative
Service from one version to another version. The original issue is opened [here](https://github.com/knative/serving/issues/12971), and the design doc is [here](https://docs.google.com/document/d/1C5iwrdC66axepm9iGVp_KrwbSnkbzfgx4iaQ_-QWc1I/edit).
When the user launch a new version of the Knative Service, Knative Serving will by default launch all the replicas for
the new version, and then scale down the replicas for the old version. With the Serving Progressive Rollout installed,
Knative Serving will launch a proportion of the replicas for the new version and scale down the old version in each stage,
until the new version fully takes over the old version.

[Installation and configuration](./DEVELOPMENT.md)

For documentation on using Knative Serving, see the [serving section](https://www.knative.dev/docs/serving/) of the [Knative documentation site](https://www.knative.dev/docs).

For documentation on the Knative Serving specification, see the [docs](https://github.com/knative/serving/tree/main/docs) folder of this repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md). For a list of all help wanted issues
across Knative, take a look at [CLOTRIBUTOR](https://clotributor.dev/search?project=knative&page=1).
