Most users are expected to use a released version of the serving-progressive-rollout, but if you're
testing or want to use a pre-released version of the release yamls, you may want to build your own
copy of the operator.

## Installing from source

You can install the Knative Operator from the source code using the
[ko](https://github.com/google/ko) build tool.

1. [Install Knative Serving](https://knative.dev/docs/install/yaml-install/serving/install-serving-with-yaml/):

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
