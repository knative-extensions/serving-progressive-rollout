Most users are expected to use a released version of the serving-progressive-rollout, but if you're
testing or want to use a pre-released version of the release yamls, you may want to build your own
copy of the operator.

## Prerequisite

1. [Install Knative Serving](https://knative.dev/docs/install/yaml-install/serving/install-serving-with-yaml/):

## Installing from releases

You can find all the available releases [here](https://github.com/knative-extensions/serving-progressive-rollout/releases).
Choose the version to install, e.g. v1.13.3. Run the following commands:

   ```
   export VERSION=v1.13.3
   kubectl apply -f https://github.com/knative-extensions/serving-progressive-rollout/releases/download/knative-${VERSION}/serving-progressive-rollout-core.yaml
   ```

## Installing from source

You can install the Knative Serving Progressive Rollout from the source code using the
[ko](https://github.com/google/ko) build tool.

1. Download the source code:

   ```
   git clone https://github.com/knative-extensions/serving-progressive-rollout.git
   ```

1. Install the CRDs and the ConfigMaps:

   ```
   kubectl apply -f config/core/300-resources
   kubectl apply -f config/core/configmaps
   ```

1. Install the serving progressive rollout:

   ```
   ko apply -f config/core/deployments/autoscaler.yaml
   ko apply -f config/core/deployments/controller.yaml
   ```

## Verification

1. To verify the installation:

   ```
   kubectl get deployment -n knative-serving
   ```

   Make sure that the deployment resources named controller and autoscaler, are up and running as below:

   ```
   NAME                   READY   UP-TO-DATE   AVAILABLE   AGE
   activator              1/1     1            1           26h
   autoscaler             1/1     1            1           26h
   autoscaler-hpa         1/1     1            1           26h
   controller             1/1     1            1           17h
   net-istio-controller   1/1     1            1           26h
   net-istio-webhook      1/1     1            1           26h
   webhook                1/1     1            1           26h
   ```

The serving progressive rollout does not change the way how you use Knative Serving. Everything happens underneath
with the user's awareness, when a new version of Knative Service is launched to replace the old one.

## Configurations

All configuration options are available in the ConfigMap named `config-rolloutorchestrator`. It is installed by default
under the namespace `knative-serving`, the same one we use for Knative Serving.

### Enable or disable the progressive rollout

* Global key: progressive-rollout-enabled, determining whether progressive rollout feature is enabled or not.
* Possible values: boolean
* Default: true

Set the key to false in order to disable the progressive rollout:
  ```yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config-rolloutorchestrator
    namespace: knative-serving
  data:
    progressive-rollout-enabled: "false"
  ```

### Configure the additional resource for each stage to roll out

* Global key: over-consumption-ratio, the percentage about how much resource more than the requested can be used
  to accomplish the rolling upgrade.
* Possible values: integer
* Default: 10

Set the key to other value, e.g. 20%, for each stage to use additionally to accomplish the rollout.
  ```yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config-rolloutorchestrator
    namespace: knative-serving
  data:
    over-consumption-ratio: "20"
  ```

### Configure the timeout for each stage in the progressive rollout process

* Global key: stage-rollout-timeout-minutes, the timeout value of minutes to use for each stage to accomplish in the
  rollout process.
* Possible values: integer
* Default: 2

Set the key to other value, e.g. 10, to change the timeout for each stage.
  ```yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config-rolloutorchestrator
    namespace: knative-serving
  data:
    stage-rollout-timeout-minutes: "10"
  ```
