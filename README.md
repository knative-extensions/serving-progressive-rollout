> [!IMPORTANT]
> This repository is now deprecated & archived. It was an experiment to explore progressive rollout strategies.

# Knative Serving Progressive Rollout

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
