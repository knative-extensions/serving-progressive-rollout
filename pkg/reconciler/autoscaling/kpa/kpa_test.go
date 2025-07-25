/*
Copyright 2023 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kpa

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	// These are the fake informers we want setup.
	fakenetworkingclient "knative.dev/networking/pkg/client/injection/client/fake"
	fakesksinformer "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/serverlessservice/fake"
	fakekubeclient "knative.dev/pkg/client/injection/kube/client/fake"
	fakefilteredpodsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod/filtered/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/core/v1/service/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/factory/filtered/fake"
	fakedynamicclient "knative.dev/pkg/injection/clients/dynamicclient/fake"
	fakeservingclient "knative.dev/serving/pkg/client/injection/client/fake"
	_ "knative.dev/serving/pkg/client/injection/ducks/autoscaling/v1alpha1/podscalable/fake"
	fakemetricinformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/metric/fake"
	fakepainformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/podautoscaler/fake"
	fakerevisioninformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/revision/fake"

	networkingclient "knative.dev/networking/pkg/client/injection/client"
	filteredinformerfactory "knative.dev/pkg/client/injection/kube/informers/factory/filtered"
	servingclient "knative.dev/serving/pkg/client/injection/client"
	"knative.dev/serving/pkg/client/injection/ducks/autoscaling/v1alpha1/podscalable"
	pareconciler "knative.dev/serving/pkg/client/injection/reconciler/autoscaling/v1alpha1/podautoscaler"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	nv1a1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	netcfg "knative.dev/networking/pkg/config"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	_ "knative.dev/pkg/metrics/testing"
	"knative.dev/pkg/observability/metrics/metricstest"
	"knative.dev/pkg/ptr"
	"knative.dev/pkg/reconciler"
	"knative.dev/pkg/system"
	_ "knative.dev/pkg/system/testing"
	_ "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/rolloutorchestrator/fake"
	_ "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/stagepodautoscaler/fake"
	"knative.dev/serving/pkg/apis/autoscaling"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/apis/serving"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	asconfig "knative.dev/serving/pkg/autoscaler/config"
	"knative.dev/serving/pkg/autoscaler/config/autoscalerconfig"
	"knative.dev/serving/pkg/autoscaler/scaling"
	"knative.dev/serving/pkg/deployment"
	"knative.dev/serving/pkg/metrics"
	areconciler "knative.dev/serving/pkg/reconciler/autoscaling"
	"knative.dev/serving/pkg/reconciler/autoscaling/config"
	"knative.dev/serving/pkg/reconciler/autoscaling/kpa/resources"
	aresources "knative.dev/serving/pkg/reconciler/autoscaling/resources"
	revisionresources "knative.dev/serving/pkg/reconciler/revision/resources"
	"knative.dev/serving/pkg/reconciler/serverlessservice/resources/names"

	recTest "knative.dev/pkg/reconciler/testing"
	testV1 "knative.dev/serving-progressive-rollout/pkg/reconciler/testing/v1"
	pkgTest "knative.dev/serving/pkg/testing"
)

const (
	defaultConcurrencyTarget = 10.0
	defaultTU                = 0.5
	gracePeriod              = 60 * time.Second
	paStableWindow           = 45 * time.Second
	progressDeadline         = 121 * time.Second
	stableWindow             = 5 * time.Minute
)

func defaultConfigMapData() map[string]string {
	return map[string]string{
		"max-scale-up-rate":                       "12.0",
		"container-concurrency-target-percentage": fmt.Sprint(defaultTU),
		"container-concurrency-target-default":    fmt.Sprint(defaultConcurrencyTarget),
		"stable-window":                           stableWindow.String(),
		"panic-window":                            "10s",
		"scale-to-zero-grace-period":              gracePeriod.String(),
		"tick-interval":                           "2s",
		"min-scale":                               "0",
		"target-burst-capacity":                   "200",
	}
}

func initialScaleZeroASConfig() *autoscalerconfig.Config {
	ac, _ := asconfig.NewConfigFromMap(defaultConfigMapData())
	ac.AllowZeroInitialScale = true
	ac.InitialScale = 0
	ac.MinScale = 0
	ac.EnableScaleToZero = true
	return ac
}

func activatorCertsNetConfig() *netcfg.Config {
	nc, _ := netcfg.NewConfigFromMap(map[string]string{
		netcfg.SystemInternalTLSKey: "enabled",
	})
	return nc
}

func defaultConfig() *config.Config {
	ac, _ := asconfig.NewConfigFromMap(defaultConfigMapData())
	deploymentConfig, _ := deployment.NewConfigFromMap(map[string]string{
		deployment.QueueSidecarImageKey: "bob",
		deployment.ProgressDeadlineKey:  progressDeadline.String(),
	})
	networkConfig, _ := netcfg.NewConfigFromMap(map[string]string{
		netcfg.SystemInternalTLSKey: "disabled",
	})

	return &config.Config{
		Autoscaler: ac,
		Deployment: deploymentConfig,
		Network:    networkConfig,
	}
}

func newConfigWatcher() configmap.Watcher {
	return configmap.NewStaticWatcher(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      asconfig.ConfigName,
		},
		Data: defaultConfigMapData(),
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      deployment.ConfigName,
		},
		Data: map[string]string{
			deployment.QueueSidecarImageKey: "covid is here",
		},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      netcfg.ConfigMapName,
		},
		Data: map[string]string{},
	})
}

func withScales(g, w int32) pkgTest.PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Status.DesiredScale, pa.Status.ActualScale = ptr.Int32(w), ptr.Int32(g)
	}
}

func metricWithDiffSvc(ns, n string) *autoscalingv1alpha1.Metric {
	m := metric(ns, n)
	m.Spec.ScrapeTarget = "something-else"
	return m
}

type metricOption func(*autoscalingv1alpha1.Metric)

func metric(ns, n string, opts ...metricOption) *autoscalingv1alpha1.Metric {
	pa := kpa(ns, n)
	m := aresources.MakeMetric(pa, names.PrivateService(n), defaultConfig().Autoscaler)
	for _, o := range opts {
		o(m)
	}
	return m
}

func sksNoConds(s *nv1a1.ServerlessService) {
	s.Status.Status = duckv1.Status{}
}

func sks(ns, n string, so ...pkgTest.SKSOption) *nv1a1.ServerlessService {
	kpa := kpa(ns, n)
	s := aresources.MakeSKS(kpa, nv1a1.SKSOperationModeServe, minActivators)
	s.Status.InitializeConditions()
	for _, opt := range so {
		opt(s)
	}
	return s
}

func markOld(pa *autoscalingv1alpha1.PodAutoscaler) {
	pa.Status.Conditions[0].LastTransitionTime.Inner.Time = time.Now().Add(-1 * time.Hour)
}

func markScaleTargetInitialized(pa *autoscalingv1alpha1.PodAutoscaler) {
	pa.Status.MarkScaleTargetInitialized()
}

func kpa(ns, n string, opts ...pkgTest.PodAutoscalerOption) *autoscalingv1alpha1.PodAutoscaler {
	rev := newTestRevision(ns, n)
	kpa := revisionresources.MakePA(rev, nil)
	kpa.Generation = 1
	kpa.Annotations[autoscaling.ClassAnnotationKey] = "kpa.autoscaling.knative.dev"
	kpa.Annotations[autoscaling.MetricAnnotationKey] = "concurrency"
	kpa.Status.InitializeConditions()
	for _, opt := range opts {
		opt(kpa)
	}
	return kpa
}

func markResourceNotOwned(rType, name string) pkgTest.PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Status.MarkResourceNotOwned(rType, name)
	}
}

func TestReconcile(t *testing.T) {
	const (
		deployName   = testRevision + "-deployment"
		defaultScale = 11
		unknownScale = scaleUnknown
		underscale   = defaultScale - 1
		overscale    = defaultScale + 1

		defaultAct = 3 // 1-10 ready pods + 200 TBC
		scaledAct  = 4 // 11 or 12 ready pods + 200 TBC
	)
	privateSvc := names.PrivateService(testRevision)

	// Set up a default deployment with the appropriate scale so that we don't
	// see patches to correct that scale.
	defaultDeployment := deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
		d.Spec.Replicas = ptr.Int32(defaultScale)
	})

	// Setup underscaled and overscaled deployment
	underscaledDeployment := deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
		d.Spec.Replicas = ptr.Int32(underscale)
	})
	overscaledDeployment := deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
		d.Spec.Replicas = ptr.Int32(overscale)
	})

	minScalePatch := clientgotesting.PatchActionImpl{
		ActionImpl: clientgotesting.ActionImpl{Namespace: testNamespace},
		Name:       deployName,
		Patch:      []byte(fmt.Sprintf(`[{"op":"replace","path":"/spec/replicas","value":%d}]`, defaultScale)),
	}

	inactiveKPAMinScale := func(g int32) *autoscalingv1alpha1.PodAutoscaler {
		return kpa(
			testNamespace, testRevision, pkgTest.WithPASKSNotReady(""), pkgTest.WithScaleTargetInitialized,
			pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
			withScales(g, unknownScale), pkgTest.WithReachabilityReachable,
			withMinScale(defaultScale), pkgTest.WithPAStatusService(testRevision),
			pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1),
		)
	}
	activatingKPAMinScale := func(g int32, opts ...pkgTest.PodAutoscalerOption) *autoscalingv1alpha1.PodAutoscaler {
		kpa := kpa(
			testNamespace, testRevision, pkgTest.WithPASKSNotReady(""),
			pkgTest.WithBufferedTraffic, withScales(g, defaultScale), pkgTest.WithReachabilityReachable,
			withMinScale(defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc),
			pkgTest.WithObservedGeneration(1),
		)
		for _, opt := range opts {
			opt(kpa)
		}
		return kpa
	}
	activeKPAMinScale := func(g, w int32) *autoscalingv1alpha1.PodAutoscaler {
		return kpa(
			testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic, markScaleTargetInitialized,
			withScales(g, w), pkgTest.WithReachabilityReachable,
			withMinScale(defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc),
			pkgTest.WithObservedGeneration(1),
		)
	}

	defaultSKS := sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady, pkgTest.WithNumActivators(defaultAct))
	defaultProxySKS := sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady, pkgTest.WithNumActivators(defaultAct))
	defaultMetric := metric(testNamespace, testRevision)

	underscaledReady := makeReadyPods(underscale, testNamespace, testRevision)
	preciseReady := makeReadyPods(defaultScale, testNamespace, testRevision)
	overscaledReady := makeReadyPods(overscale, testNamespace, testRevision)
	defaultReady := makeReadyPods(1, testNamespace, testRevision)[0]

	type deciderKey struct{}
	type asConfigKey struct{}
	type netConfigKey struct{}

	retryAttempted := false

	// Note: due to how KPA reconciler works we are dependent on the
	// two constant objects above, which means, that all tests must share
	// the same namespace and revision name.
	table := recTest.TableTest{{
		Name: "bad workqueue key, Part I",
		Key:  "too/many/parts",
	}, {
		Name: "bad workqueue key, Part II",
		Key:  "too-few-parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}, {
		Name: "steady state",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic,
				markScaleTargetInitialized, pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment, defaultReady},
	}, {
		Name: "status update retry",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithPAStatusService(testRevision),
				withScales(0, defaultScale)),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment, defaultReady},
		WithReactors: []clientgotesting.ReactionFunc{
			func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				if retryAttempted || !action.Matches("update", "podautoscalers") || action.GetSubresource() != "status" {
					return false, nil, nil
				}
				retryAttempted = true
				return true, nil, apierrors.NewConflict(v1.Resource("foo"), "bar", errors.New("foo"))
			},
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithTraffic,
				markScaleTargetInitialized, pkgTest.WithPASKSReady,
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
		}, {
			Object: kpa(testNamespace, testRevision, pkgTest.WithTraffic,
				markScaleTargetInitialized, pkgTest.WithPASKSReady,
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "failure-creating-metric-object",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithTraffic, pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady),
			defaultDeployment, defaultReady},
		WithReactors: []clientgotesting.ReactionFunc{
			recTest.InduceFailure("create", "metrics"),
		},
		WantCreates: []runtime.Object{
			metric(testNamespace, testRevision),
		},
		WantEvents: []string{
			recTest.Eventf(corev1.EventTypeWarning, "InternalError",
				`error reconciling Metric: error creating metric: inducing failure for create metrics`),
		},
		WantErr: true,
	}, {
		Name: "failure-updating-metric-object",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithTraffic, pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady),
			defaultDeployment,
			metricWithDiffSvc(testNamespace, testRevision), defaultReady},
		WithReactors: []clientgotesting.ReactionFunc{
			recTest.InduceFailure("update", "metrics"),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: metric(testNamespace, testRevision),
		}},
		WantEvents: []string{
			recTest.Eventf(corev1.EventTypeWarning, "InternalError",
				`error reconciling Metric: error updating metric: inducing failure for update metrics`),
		},
		WantErr: true,
	}, {
		Name: "create metric",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithTraffic, markScaleTargetInitialized,
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision)),
			defaultSKS, defaultDeployment, defaultReady},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithTraffic, markScaleTargetInitialized,
				withScales(1, defaultScale), pkgTest.WithPASKSReady, pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1)),
		}},
		WantCreates: []runtime.Object{
			metric(testNamespace, testRevision),
		},
	}, {
		Name: "scale up deployment",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic, markScaleTargetInitialized,
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithObservedGeneration(1)),
			defaultSKS,
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision), defaultReady},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: testNamespace,
			},
			Name:  deployName,
			Patch: []byte(`[{"op":"add","path":"/spec/replicas","value":11}]`),
		}},
	}, {
		Name: "scale up deployment failure",
		Key:  key,
		WithReactors: []clientgotesting.ReactionFunc{
			recTest.InduceFailure("patch", "deployments"),
		},
		WantErr: true,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithTraffic, pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			defaultSKS,
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision), defaultReady},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: testNamespace,
			},
			Name:  deployName,
			Patch: []byte(`[{"op":"add","path":"/spec/replicas","value":11}]`),
		}},
		WantEvents: []string{
			recTest.Eventf(corev1.EventTypeWarning, "InternalError",
				`error scaling target: failed to apply scale 11 to scale target test-revision-deployment: inducing failure for patch deployments`),
		},
	}, {
		Name: "pa activates",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithScaleTargetInitialized, pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
				withScales(0, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc)),
			// SKS is ready here, since its endpoints are populated with Activator endpoints.
			sks(testNamespace, testRevision, pkgTest.WithProxyMode, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady),
			metric(testNamespace, testRevision),
			defaultDeployment,
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, pkgTest.WithSKSReady,
				pkgTest.WithDeployRef(deployName)),
		}},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithScaleTargetInitialized, pkgTest.WithBufferedTraffic, withScales(0, defaultScale),
				pkgTest.WithPASKSReady, pkgTest.WithPAMetricsService(privateSvc),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "sks is still not ready",
		Key:  key,
		Objects: append([]runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithTraffic, pkgTest.WithPAMetricsService(privateSvc),
				withScales(0, defaultScale), pkgTest.WithPAStatusService(testRevision)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithPubService,
				pkgTest.WithPrivateService, pkgTest.WithNumActivators(scaledAct)),
			metric(testNamespace, testRevision),
			defaultDeployment},
			preciseReady...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, withScales(defaultScale, defaultScale),
				pkgTest.WithPASKSNotReady(""), pkgTest.WithTraffic, markScaleTargetInitialized,
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "sks becomes ready",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSNotReady(""),
				pkgTest.WithBufferedTraffic, pkgTest.WithPAMetricsService(privateSvc), withScales(0, unknownScale),
				pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady, pkgTest.WithNumActivators(minActivators)),
			defaultDeployment, metric(testNamespace, testRevision),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithBufferedTraffic, pkgTest.WithPAMetricsService(privateSvc), withScales(0, defaultScale),
				pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "kpa does not become ready without minScale endpoints when reachable",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withMinScale(2), withScales(1, defaultScale),
				pkgTest.WithReachabilityReachable, pkgTest.WithPAMetricsService(privateSvc)),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment, defaultReady},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady,
				pkgTest.WithBufferedTraffic, withMinScale(2), pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithReachabilityReachable,
				pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "kpa does not become ready without minScale endpoints when reachability is unknown",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withMinScale(2), withScales(1, defaultScale),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithReachabilityUnknown),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment,
			defaultReady},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady,
				pkgTest.WithBufferedTraffic, withMinScale(2), pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithReachabilityUnknown,
				pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "kpa becomes ready without minScale endpoints when unreachable",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withMinScale(2), withScales(1, defaultScale),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithReachabilityUnreachable),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment, defaultReady},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady,
				pkgTest.WithTraffic, markScaleTargetInitialized, withMinScale(2), pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithReachabilityUnreachable,
				pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "kpa becomes ready with minScale endpoints when reachable",
		Key:  key,
		Objects: append([]runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithBufferedTraffic, withMinScale(2), pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale), pkgTest.WithReachabilityReachable),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment,
		}, makeReadyPods(2, testNamespace, testRevision)...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady,
				pkgTest.WithTraffic, markScaleTargetInitialized, withMinScale(2), pkgTest.WithPAMetricsService(privateSvc),
				withScales(2, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithReachabilityReachable,
				pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "kpa becomes ready with minScale endpoints when reachability is unknown",
		Key:  key,
		Objects: append([]runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithBufferedTraffic, withMinScale(2), pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale), pkgTest.WithReachabilityUnknown),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment,
		}, makeReadyPods(2, testNamespace, testRevision)...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady,
				pkgTest.WithTraffic, markScaleTargetInitialized, withMinScale(2), pkgTest.WithPAMetricsService(privateSvc),
				withScales(2, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithReachabilityUnknown,
				pkgTest.WithObservedGeneration(1)),
		}},
	}, {
		Name: "sks does not exist",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithTraffic, pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale)),
			defaultDeployment,
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			// SKS does not exist, so we're just creating and have no status.
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSNotReady("No Private Service Name"),
				pkgTest.WithBufferedTraffic, pkgTest.WithPAMetricsService(privateSvc), withScales(0, defaultScale),
				pkgTest.WithObservedGeneration(1)),
		}},
		WantCreates: []runtime.Object{
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithNumActivators(minActivators), pkgTest.WithProxyMode, sksNoConds),
		},
	}, {
		Name: "sks is out of whack",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady,
				withScales(0, defaultScale), pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithTraffic),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef("bar"), pkgTest.WithPubService, pkgTest.WithPrivateService),
			metric(testNamespace, testRevision),
			defaultDeployment,
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			// SKS just got updated and we don't have up to date status.
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSNotReady(""),
				pkgTest.WithBufferedTraffic, withScales(0, defaultScale), pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1)),
		}},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, pkgTest.WithPubService, pkgTest.WithPrivateService,
				pkgTest.WithDeployRef(deployName)),
		}},
	}, {
		Name: "sks cannot be created",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithTraffic, pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale), pkgTest.WithObservedGeneration(1)),
			metric(testNamespace, testRevision),
			defaultDeployment,
		},
		WithReactors: []clientgotesting.ReactionFunc{
			recTest.InduceFailure("create", "serverlessservices"),
		},
		WantErr: true,
		WantCreates: []runtime.Object{
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithNumActivators(minActivators), sksNoConds),
		},
		WantEvents: []string{
			recTest.Eventf(corev1.EventTypeWarning, "InternalError",
				"error reconciling SKS: error creating SKS test-revision: inducing failure for create serverlessservices"),
		},
	}, {
		Name: "sks cannot be updated",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withScales(1, defaultScale), pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithTraffic, pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithProxyMode, pkgTest.WithDeployRef("bar")),
			metric(testNamespace, testRevision),
			defaultDeployment,
		},
		WithReactors: []clientgotesting.ReactionFunc{
			recTest.InduceFailure("update", "serverlessservices"),
		},
		WantErr: true,
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, pkgTest.WithProxyMode, pkgTest.WithDeployRef(deployName), pkgTest.WithNumActivators(minActivators)),
		}},
		WantEvents: []string{
			recTest.Eventf(corev1.EventTypeWarning, "InternalError", "error reconciling SKS: error updating SKS test-revision: inducing failure for update serverlessservices"),
		},
	}, {
		Name: "sks is disowned",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withScales(1, defaultScale), pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithTraffic),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady,
				pkgTest.WithSKSOwnersRemoved),
			metric(testNamespace, testRevision),
			defaultDeployment,
		},
		WantErr: true,
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, withScales(1, defaultScale),
				pkgTest.WithPAMetricsService(privateSvc), markResourceNotOwned("ServerlessService", testRevision), pkgTest.WithObservedGeneration(1)),
		}},
		WantEvents: []string{
			recTest.Eventf(corev1.EventTypeWarning, "InternalError", "error reconciling SKS: PA: test-revision does not own SKS: test-revision"),
		},
	}, {
		Name: "metric is disowned",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withScales(1, defaultScale), pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithTraffic),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady),
			metric(testNamespace, testRevision, pkgTest.WithMetricOwnersRemoved),
			defaultDeployment,
		},
		WantErr: true,
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, withScales(1, defaultScale),
				pkgTest.WithPAMetricsService(privateSvc), markResourceNotOwned("Metric", testRevision), pkgTest.WithObservedGeneration(1)),
		}},
		WantEvents: []string{
			recTest.Eventf(corev1.EventTypeWarning, "InternalError", `error reconciling Metric: PA: test-revision does not own Metric: test-revision`),
		},
	}, {
		Name: "steady not serving",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 0 /* desiredScale */, 0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithScaleTargetInitialized, withScales(0, 0),
				pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
				pkgTest.WithPASKSReady, markOld, pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady),
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
				d.Spec.Replicas = ptr.Int32(0)
			}),
		},
	}, {
		Name: "steady not serving (scale to zero)",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 0 /* desiredScale */, 0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithScaleTargetInitialized, withScales(0, 0),
				pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
				pkgTest.WithPASKSReady, markOld, pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady),
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision),
		},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: testNamespace,
			},
			Name:  deployName,
			Patch: []byte(`[{"op":"add","path":"/spec/replicas","value":0}]`),
		}},
	}, {
		Name: "from serving to proxy",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 0 /* desiredScale */, 0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic, markOld,
				withScales(0, 0), pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc)),
			defaultSKS,
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision), defaultReady},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, markScaleTargetInitialized, withScales(1, 0),
				pkgTest.WithPASKSReady, pkgTest.WithPAMetricsService(privateSvc),
				pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc),
				pkgTest.WithObservedGeneration(1)),
		}},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: defaultProxySKS,
		}},
	}, {
		Name: "scaling to 0, but not stable for long enough, so no-op",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 0 /* desiredScale */, 0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic,
				markScaleTargetInitialized, withScales(1, 1),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1)),
			defaultSKS,
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision), defaultReady},
	}, {
		Name: "activation failure",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 0 /* desiredScale */, 0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithBufferedTraffic, markOld,
				pkgTest.WithPAStatusService(testRevision), withScales(0, 0),
				pkgTest.WithPAMetricsService(privateSvc)),
			defaultSKS,
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision), defaultReady},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, markScaleTargetInitialized, pkgTest.WithPASKSReady, pkgTest.WithPAMetricsService(privateSvc),
				pkgTest.WithNoTraffic("TimedOut", "The target could not be activated."), withScales(1, 0),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc),
				pkgTest.WithObservedGeneration(1)),
		}},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: defaultProxySKS,
		}},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{
				Namespace: testNamespace,
			},
			Name:  deployName,
			Patch: []byte(`[{"op":"add","path":"/spec/replicas","value":0}]`),
		}},
	}, {
		Name: "want=-1, underscaled, PA inactive",
		// No-op
		Key: key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, unknownScale, 0 /* ebc */)),
		Objects: append([]runtime.Object{
			inactiveKPAMinScale(underscale), underscaledDeployment,
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode,
				pkgTest.WithPubService, pkgTest.WithPrivateService, pkgTest.WithNumActivators(defaultAct)),
			defaultMetric,
		}, underscaledReady...),
	}, {
		Name: "want=1, underscaled, PA inactive",
		// Status -> Activating and Deployment has to be patched.
		Key: key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 1, 0 /* ebc */)),
		Objects: append([]runtime.Object{
			inactiveKPAMinScale(underscale), underscaledDeployment,
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode,
				pkgTest.WithPubService, pkgTest.WithPrivateService, pkgTest.WithNumActivators(defaultAct)),
			defaultMetric,
		}, underscaledReady...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: activatingKPAMinScale(underscale, pkgTest.WithScaleTargetInitialized),
		}},
		WantPatches: []clientgotesting.PatchActionImpl{
			minScalePatch,
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName),
				pkgTest.WithPubService, pkgTest.WithPrivateService, pkgTest.WithNumActivators(defaultAct)),
		}},
	}, {
		Name: "underscaled, PA activating",
		// Scale to `minScale`
		Key: key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 2 /*autoscaler desired scale*/, 0 /* ebc */)),
		Objects: append([]runtime.Object{
			activatingKPAMinScale(underscale, pkgTest.WithPASKSReady), underscaledDeployment,
			defaultSKS, defaultMetric,
		}, underscaledReady...),
		WantPatches: []clientgotesting.PatchActionImpl{
			minScalePatch,
		},
	}, {
		Name: "underscaled, PA active",
		// Mark PA "activating"
		Key: key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, 0 /* ebc */)),
		Objects: append([]runtime.Object{
			activeKPAMinScale(underscale, defaultScale), underscaledDeployment,
			defaultSKS, defaultMetric,
		}, underscaledReady...),
		WantPatches: []clientgotesting.PatchActionImpl{
			minScalePatch,
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: activatingKPAMinScale(underscale, markScaleTargetInitialized, pkgTest.WithPASKSReady),
		}},
	}, {
		// Scale to `minScale` and mark PA "active"
		Name: "overscaled, PA inactive",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 0 /*wantScale*/, 0 /* ebc */)),
		Objects: append([]runtime.Object{
			inactiveKPAMinScale(overscale), overscaledDeployment,
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
			defaultMetric,
		}, overscaledReady...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
		}},
		WantPatches: []clientgotesting.PatchActionImpl{
			minScalePatch,
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: activeKPAMinScale(overscale, defaultScale),
		}},
	}, {
		Name: "overscaled, PA activating",
		// Scale to `minScale` and mark PA "active"
		Key: key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 1 /*wantScale*/, 0 /* ebc */)),
		Objects: append([]runtime.Object{
			inactiveKPAMinScale(overscale), overscaledDeployment,
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
			defaultMetric,
		}, overscaledReady...),
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
		}},
		WantPatches: []clientgotesting.PatchActionImpl{
			minScalePatch,
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: activeKPAMinScale(overscale, defaultScale),
		}},
	}, {
		Name: "over maxScale for real, PA active",
		// No-op.
		Key: key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, overscale, /*want more than minScale*/
				0 /* ebc */)),
		Objects: append([]runtime.Object{
			activeKPAMinScale(overscale, overscale), overscaledDeployment,
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
			defaultMetric,
		}, overscaledReady...),
	}, {
		Name: "over maxScale, need to scale down, PA active",
		// No-op.
		Key: key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 1, /*less than minScale*/
				0 /* ebc */)),
		Objects: append([]runtime.Object{
			activeKPAMinScale(overscale, overscale), overscaledDeployment,
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
			defaultMetric,
		}, overscaledReady...),
		WantPatches: []clientgotesting.PatchActionImpl{
			minScalePatch,
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: activeKPAMinScale(overscale, defaultScale),
		}},
	}, {
		Name: "scaled-to-0-no-scale-data",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, unknownScale, /* desiredScale */
				0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithScaleTargetInitialized, pkgTest.WithPASKSReady,
				pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."), pkgTest.WithPAMetricsService(privateSvc),
				withScales(0, -1), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName),
				pkgTest.WithProxyMode, pkgTest.WithSKSReady),
			metric(testNamespace, testRevision),
			defaultDeployment,
		},
	}, {
		Name: "steady not enough capacity",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, /* desiredScale */
				-42 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic,
				markScaleTargetInitialized, pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			defaultProxySKS,
			metric(testNamespace, testRevision),
			defaultDeployment, defaultReady},
	}, {
		Name: "traffic increased, no longer enough burst capacity",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, /* desiredScale */
				-18 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic, markScaleTargetInitialized,
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady,
				pkgTest.WithNumActivators(2)),
			metric(testNamespace, testRevision),
			defaultDeployment, defaultReady},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, pkgTest.WithSKSReady,
				pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithNumActivators(minActivators+1)),
		}},
	}, {
		Name: "traffic decreased, now we have enough burst capacity",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, /* desiredScale */
				1 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic, markScaleTargetInitialized,
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			defaultProxySKS,
			metric(testNamespace, testRevision),
			defaultDeployment,
			defaultReady},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: defaultSKS,
		}},
	}, {
		Name: "initial scale > minScale, have not reached initial scale, PA still activating",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, /* desiredScale */
				-42 /* ebc */)),
		Objects: append([]runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithBufferedTraffic,
				withScales(defaultScale, defaultScale), pkgTest.WithReachabilityReachable,
				withMinScale(defaultScale), withInitialScale(), pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
			defaultMetric, defaultDeployment,
		}, makeReadyPods(defaultScale, testNamespace, testRevision)...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithBufferedTraffic,
				withScales(defaultScale, 20), pkgTest.WithReachabilityReachable,
				withMinScale(defaultScale), withInitialScale(), pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc),
				pkgTest.WithObservedGeneration(1),
			),
		}},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{Namespace: testNamespace},
			Name:       deployName,
			Patch:      []byte(fmt.Sprintf(`[{"op":"replace","path":"/spec/replicas","value":%d}]`, 20)),
		}},
	}, {
		Name: "initial scale reached, mark PA as active",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, /* desiredScale */
				-42 /* ebc */)),
		Objects: append([]runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithBufferedTraffic,
				withScales(20, defaultScale), withInitialScale(), pkgTest.WithReachabilityReachable,
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc),
			),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady, pkgTest.WithNumActivators(scaledAct)),
			defaultMetric,
			deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
				d.Spec.Replicas = ptr.Int32(defaultScale)
			}),
		}, makeReadyPods(20, testNamespace, testRevision)...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic,
				markScaleTargetInitialized, withScales(20, 20), withInitialScale(), pkgTest.WithReachabilityReachable,
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1),
			),
		}},
		WantPatches: []clientgotesting.PatchActionImpl{{
			ActionImpl: clientgotesting.ActionImpl{Namespace: testNamespace},
			Name:       deployName,
			Patch:      []byte(fmt.Sprintf(`[{"op":"replace","path":"/spec/replicas","value":%d}]`, 20)),
		}},
	}, {
		Name: "initial scale zero: scale to zero",
		Key:  key,
		Ctx: context.WithValue(context.WithValue(context.Background(), asConfigKey{}, initialScaleZeroASConfig()), deciderKey{},
			decider(testNamespace, testRevision, -1, /* desiredScale */
				0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withScales(0, -1), pkgTest.WithReachabilityReachable,
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithPASKSNotReady(noPrivateServiceName)),
			// SKS won't be ready bc no ready endpoints, but private service name will be populated.
			sks(testNamespace, testRevision, pkgTest.WithProxyMode, pkgTest.WithDeployRef(deployName), pkgTest.WithPrivateService, pkgTest.WithPubService),
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
				d.Spec.Replicas = ptr.Int32(0)
			}),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, markScaleTargetInitialized,
				pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
				withScales(0, -1), pkgTest.WithReachabilityReachable,
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1),
				pkgTest.WithPASKSNotReady(""), pkgTest.WithPAStatusService(testRevision),
			),
		}},
	}, {
		Name: "initial scale zero: sks ServiceName empty",
		Key:  key,
		Ctx: context.WithValue(context.WithValue(context.Background(), asConfigKey{}, initialScaleZeroASConfig()), deciderKey{},
			decider(testNamespace, testRevision, -1, /* desiredScale */
				0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, withScales(0, -1), pkgTest.WithReachabilityReachable,
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithPASKSNotReady(noPrivateServiceName)),
			// SKS won't be ready bc no ready endpoints, but private service name will be populated.
			sks(testNamespace, testRevision, pkgTest.WithProxyMode, pkgTest.WithDeployRef(deployName), pkgTest.WithPrivateService),
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
				d.Spec.Replicas = ptr.Int32(0)
			}),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision,
				pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
				withScales(0, -1), pkgTest.WithReachabilityReachable,
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1),
				pkgTest.WithPASKSNotReady(""),
			),
		}},
	}, {
		Name: "initial scale zero: stay at zero",
		Key:  key,
		Ctx: context.WithValue(context.WithValue(context.Background(), asConfigKey{}, initialScaleZeroASConfig()), deciderKey{},
			decider(testNamespace, testRevision, -1 /* desiredScale */, 0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markScaleTargetInitialized, withScales(0, scaleUnknown),
				pkgTest.WithReachabilityReachable, pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithPASKSNotReady(""),
			),
			sks(testNamespace, testRevision, pkgTest.WithProxyMode, pkgTest.WithDeployRef(deployName), pkgTest.WithPrivateService),
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
				d.Spec.Replicas = ptr.Int32(0)
			}),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithPASKSNotReady(""), pkgTest.WithBufferedTraffic, markScaleTargetInitialized,
				withScales(0, scaleUnknown), pkgTest.WithReachabilityReachable,
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1),
			),
		}},
	}, {
		Name: "initial scale zero: scale to greater than zero",
		Key:  key,
		Ctx: context.WithValue(context.WithValue(context.Background(), asConfigKey{}, initialScaleZeroASConfig()), deciderKey{},
			decider(testNamespace, testRevision, 2, /* desiredScale */
				-42 /* ebc */)),
		Objects: append([]runtime.Object{
			kpa(testNamespace, testRevision, markScaleTargetInitialized, withScales(2, 2),
				pkgTest.WithReachabilityReachable, pkgTest.WithPAStatusService(testRevision), pkgTest.WithPAMetricsService(privateSvc),
				pkgTest.WithPASKSReady,
			),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady, pkgTest.WithNumActivators(defaultAct)),
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
				d.Spec.Replicas = ptr.Int32(2)
			}),
		}, makeReadyPods(2, testNamespace, testRevision)...),
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, pkgTest.WithTraffic, pkgTest.WithPASKSReady, markScaleTargetInitialized,
				withScales(2, 2), pkgTest.WithReachabilityReachable, pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1),
			),
		}},
	}, {
		Name: "we have enough burst capacity, but keep proxy mode as activator CA is enabled",
		Key:  key,
		Ctx: context.WithValue(context.WithValue(context.Background(), netConfigKey{}, activatorCertsNetConfig()), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, /* desiredScale */
				1 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic, markScaleTargetInitialized,
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			defaultProxySKS,
			metric(testNamespace, testRevision),
			defaultDeployment,
			defaultReady},
		// No update from ProxySKS.
	}, {
		Name: "we have enough burst capacity, but switch to keep proxy mode as activator CA is turned on",
		Key:  key,
		Ctx: context.WithValue(context.WithValue(context.Background(), netConfigKey{}, activatorCertsNetConfig()), deciderKey{},
			decider(testNamespace, testRevision, defaultScale, /* desiredScale */
				1 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic, markScaleTargetInitialized,
				pkgTest.WithPAMetricsService(privateSvc), withScales(1, defaultScale),
				pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment,
			defaultReady},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: defaultProxySKS,
		}},
	}}

	table.Test(t, testV1.MakeFactory(func(ctx context.Context, listers *testV1.Listers, _ configmap.Watcher) controller.Reconciler {
		retryAttempted = false
		ctx = podscalable.WithDuck(ctx)

		fakeDeciders := newTestDeciders()
		// TODO(vagababov): see if we can get rid of the static piece of configuration and
		// constant namespace and revision names.

		// Make new decider if it's not in the context
		if d := ctx.Value(deciderKey{}); d == nil {
			decider := resources.MakeDecider(
				kpa(testNamespace, testRevision), defaultConfig().Autoscaler)
			decider.Status.DesiredScale = defaultScale
			decider.Generation = 2112
			fakeDeciders.Create(ctx, decider)
		} else {
			fakeDeciders.Create(ctx, d.(*scaling.Decider))
		}

		testConfigs := defaultConfig()
		if asConfig := ctx.Value(asConfigKey{}); asConfig != nil {
			testConfigs.Autoscaler = asConfig.(*autoscalerconfig.Config)
		}
		if netConfig := ctx.Value(netConfigKey{}); netConfig != nil {
			testConfigs.Network = netConfig.(*netcfg.Config)
		}
		psf := podscalable.Get(ctx)
		scaler := newScaler(ctx, psf, func(interface{}, time.Duration) {})
		scaler.activatorProbe = func(*autoscalingv1alpha1.PodAutoscaler, http.RoundTripper) (bool, error) { return true, nil }
		r := &Reconciler{
			Base: &areconciler.Base{
				Client:           servingclient.Get(ctx),
				NetworkingClient: networkingclient.Get(ctx),
				SKSLister:        listers.GetServerlessServiceLister(),
				MetricLister:     listers.GetMetricLister(),
			},
			podsLister: listers.GetPodsLister(),
			deciders:   fakeDeciders,
			scaler:     scaler,
			spaLister:  listers.GetStagePodAutoscalerLister(),
		}
		return pareconciler.NewReconciler(ctx, logging.FromContext(ctx),
			servingclient.Get(ctx), listers.GetPodAutoscalerLister(),
			controller.GetEventRecorder(ctx), r, autoscaling.KPA,
			controller.Options{
				ConfigStore: &testConfigStore{config: testConfigs},
			})
	}))
}

type deploymentOption func(*appsv1.Deployment)

func deploy(namespace, name string, opts ...deploymentOption) *appsv1.Deployment {
	s := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-deployment",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "b",
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 42,
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func TestGlobalResyncOnUpdateAutoscalerConfigMap(t *testing.T) {
	ctx, cancel, informers := recTest.SetupFakeContextWithCancel(t, func(ctx context.Context) context.Context {
		return filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	})
	watcher := &configmap.ManualWatcher{Namespace: system.Namespace()}

	fakeDeciders := newTestDeciders()
	ctl := NewController(ctx, watcher, fakeDeciders)

	// Load default config
	watcher.OnChange(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      asconfig.ConfigName,
			Namespace: system.Namespace(),
		},
		Data: defaultConfigMapData(),
	})
	watcher.OnChange(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployment.ConfigName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{
			deployment.QueueSidecarImageKey: "i'm on a bike",
		},
	})
	watcher.OnChange(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netcfg.ConfigMapName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{},
	})

	grp := errgroup.Group{}
	waitInformers, err := recTest.RunAndSyncInformers(ctx, informers...)
	if err != nil {
		t.Fatal("Failed to start informers:", err)
	}
	defer func() {
		cancel()
		if err := grp.Wait(); err != nil {
			t.Error("Wait() =", err)
		}
		waitInformers()
	}()

	if err := watcher.Start(ctx.Done()); err != nil {
		t.Fatal("failed to start configmap watcher:", err)
	}

	grp.Go(func() error { controller.StartAll(ctx, ctl); return nil })

	rev := newTestRevision(testNamespace, testRevision)
	newDeployment(ctx, t, fakedynamicclient.Get(ctx), testRevision+"-deployment", 3)

	kpa := revisionresources.MakePA(rev, nil)
	sks := aresources.MakeSKS(kpa, nv1a1.SKSOperationModeServe, minActivators)
	sks.Status.PrivateServiceName = "bogus"
	sks.Status.InitializeConditions()

	fakenetworkingclient.Get(ctx).NetworkingV1alpha1().ServerlessServices(testNamespace).Create(ctx, sks, metav1.CreateOptions{})
	fakesksinformer.Get(ctx).Informer().GetIndexer().Add(sks)

	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(ctx, kpa, metav1.CreateOptions{})
	fakepainformer.Get(ctx).Informer().GetIndexer().Add(kpa)

	// Wait for decider to be created.
	if decider, err := pollDeciders(fakeDeciders, testNamespace, testRevision, nil); err != nil {
		t.Fatal("Failed to get decider:", err)
	} else if got, want := decider.Spec.TargetValue, defaultConcurrencyTarget*defaultTU; got != want {
		t.Fatalf("TargetValue = %f, want %f", got, want)
	}

	const concurrencyTargetAfterUpdate = 100.0
	data := defaultConfigMapData()
	data["container-concurrency-target-default"] = fmt.Sprint(concurrencyTargetAfterUpdate)
	watcher.OnChange(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      asconfig.ConfigName,
			Namespace: system.Namespace(),
		},
		Data: data,
	})

	// Wait for decider to be updated with the new values from the configMap.
	want := concurrencyTargetAfterUpdate * defaultTU
	cond := func(d *scaling.Decider) bool {
		return d.Spec.TargetValue == want
	}
	if decider, err := pollDeciders(fakeDeciders, testNamespace, testRevision, cond); err != nil {
		t.Fatal("Failed to get decider:", err)
	} else if got := decider.Spec.TargetValue; got != want {
		t.Fatalf("TargetValue = %f, want %f", got, want)
	}
}

func TestReconcileDeciderCreatesAndDeletes(t *testing.T) {
	ctx, cancel, informers := recTest.SetupFakeContextWithCancel(t, func(ctx context.Context) context.Context {
		return filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	})
	fakeDeciders := newTestDeciders()
	ctl := NewController(ctx, newConfigWatcher(), fakeDeciders)

	wf, err := recTest.RunAndSyncInformers(ctx, informers...)
	if err != nil {
		cancel()
		t.Fatal("Failed to start informers:", err)
	}

	var eg errgroup.Group
	defer func() {
		cancel()
		wf()
		eg.Wait()
	}()

	rev := newTestRevision(testNamespace, testRevision)
	fakeservingclient.Get(ctx).ServingV1().Revisions(testNamespace).Create(ctx, rev, metav1.CreateOptions{})

	pod := makeReadyPods(1, testNamespace, testRevision)[0].(*corev1.Pod)
	fakekubeclient.Get(ctx).CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})

	newDeployment(ctx, t, fakedynamicclient.Get(ctx), testRevision+"-deployment", 3)

	kpa := revisionresources.MakePA(rev, nil)
	sks := sks(testNamespace, testRevision, pkgTest.WithDeployRef(kpa.Spec.ScaleTargetRef.Name), pkgTest.WithSKSReady)
	fakenetworkingclient.Get(ctx).NetworkingV1alpha1().ServerlessServices(testNamespace).Create(ctx, sks, metav1.CreateOptions{})
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(ctx, kpa, metav1.CreateOptions{})

	wait.PollUntilContextTimeout(ctx, 10*time.Millisecond, 5*time.Second, true, func(context.Context) (bool, error) {
		_, err := fakepainformer.Get(ctx).Lister().PodAutoscalers(testNamespace).Get(kpa.Name)
		if err != nil && apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		return true, nil
	})

	// Start controller after creating initial resources so it observes a steady state.
	eg.Go(func() error { return ctl.RunContext(ctx, 1) })

	select {
	case <-time.After(5 * time.Second):
		t.Error("Didn't observe call to create Decider.")
	case <-fakeDeciders.createCall:
		// What we want.
	}

	// The ReconcileKind call hasn't finished yet at the point where the Decider is created,
	// so give it more time to finish before checking the PA for IsReady().
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Millisecond, 5*time.Second, true, func(context.Context) (bool, error) {
		newKPA, err := fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(kpa.Namespace).Get(
			ctx, kpa.Name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		return newKPA.IsReady() && newKPA.Status.GetDesiredScale() == 1, nil
	}); err != nil {
		t.Fatal("PA failed to become ready:", err)
	}

	fakeservingclient.Get(ctx).ServingV1().Revisions(testNamespace).Delete(ctx, testRevision, metav1.DeleteOptions{})
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Delete(ctx, testRevision, metav1.DeleteOptions{})

	select {
	case <-time.After(5 * time.Second):
		t.Error("Didn't observe call to delete Decider.")
	case <-fakeDeciders.deleteCall:
		// What we want.
	}

	if fakeDeciders.deleteBeforeCreate.Load() {
		t.Fatal("Deciders.Delete ran before OnPresent")
	}
}

func TestUpdate(t *testing.T) {
	ctx, cancel, _ := recTest.SetupFakeContextWithCancel(t, func(ctx context.Context) context.Context {
		return filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	})
	t.Cleanup(cancel)

	fakeDeciders := newTestDeciders()
	ctl := NewController(ctx, newConfigWatcher(), fakeDeciders)

	rev := newTestRevision(testNamespace, testRevision)
	fakeservingclient.Get(ctx).ServingV1().Revisions(testNamespace).Create(ctx, rev, metav1.CreateOptions{})
	fakerevisioninformer.Get(ctx).Informer().GetIndexer().Add(rev)

	newDeployment(ctx, t, fakedynamicclient.Get(ctx), testRevision+"-deployment", 3)

	pod := makeReadyPods(1, testNamespace, testRevision)[0].(*corev1.Pod)
	fakekubeclient.Get(ctx).CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
	fakefilteredpodsinformer.Get(ctx, serving.RevisionUID).Informer().GetIndexer().Add(pod)

	kpa := revisionresources.MakePA(rev, nil)
	kpa.SetDefaults(context.Background())
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(ctx, kpa, metav1.CreateOptions{})
	fakepainformer.Get(ctx).Informer().GetIndexer().Add(kpa)

	metric := aresources.MakeMetric(kpa, "", defaultConfig().Autoscaler)
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().Metrics(testNamespace).Create(ctx, metric, metav1.CreateOptions{})
	fakemetricinformer.Get(ctx).Informer().GetIndexer().Add(metric)

	sks := sks(testNamespace, testRevision, pkgTest.WithDeployRef(kpa.Spec.ScaleTargetRef.Name),
		pkgTest.WithSKSReady)
	fakenetworkingclient.Get(ctx).NetworkingV1alpha1().ServerlessServices(testNamespace).Create(ctx, sks, metav1.CreateOptions{})
	fakesksinformer.Get(ctx).Informer().GetIndexer().Add(sks)

	decider := resources.MakeDecider(kpa, defaultConfig().Autoscaler)

	// The Reconciler won't do any work until it becomes the leader.
	if la, ok := ctl.Reconciler.(reconciler.LeaderAware); ok {
		la.Promote(reconciler.UniversalBucket(), func(reconciler.Bucket, types.NamespacedName) {})
	}

	// Wait for the Reconcile to complete.
	if err := ctl.Reconciler.Reconcile(ctx, testNamespace+"/"+testRevision); err != nil {
		t.Error("Reconcile() =", err)
	}

	if count := fakeDeciders.createCallCount.Load(); count != 1 {
		t.Fatalf("Deciders.Create called %d times instead of once", count)
	}

	// Verify decider shape.
	if got, want := fakeDeciders.decider, decider; !cmp.Equal(got, want) {
		t.Error("decider mismatch: diff(+got, -want):", cmp.Diff(got, want))
	}

	newKPA, err := fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(kpa.Namespace).Get(
		ctx, kpa.Name, metav1.GetOptions{})
	if err != nil {
		t.Error("Get() =", err)
	}
	if cond := newKPA.Status.GetCondition("Ready"); cond == nil || cond.Status != "True" {
		t.Errorf("GetCondition(Ready) = %v, wanted True", cond)
	}

	// Update the KPA container concurrency.
	kpa.Spec.ContainerConcurrency = 2
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Update(ctx, kpa, metav1.UpdateOptions{})
	fakepainformer.Get(ctx).Informer().GetIndexer().Update(kpa)

	if err := ctl.Reconciler.Reconcile(ctx, testNamespace+"/"+testRevision); err != nil {
		t.Error("Reconcile() =", err)
	}

	if fakeDeciders.updateCallCount.Load() == 0 {
		t.Fatal("Deciders.Update was not called")
	}
}

func TestControllerCreateError(t *testing.T) {
	ctx, cancel, infs := recTest.SetupFakeContextWithCancel(t, func(ctx context.Context) context.Context {
		return filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	})
	waitInformers, err := recTest.RunAndSyncInformers(ctx, infs...)
	if err != nil {
		t.Fatal("Error starting up informers:", err)
	}
	defer func() {
		cancel()
		waitInformers()
	}()

	want := apierrors.NewBadRequest("asdf")

	ctl := NewController(ctx, newConfigWatcher(),
		&failingDeciders{
			getErr:    apierrors.NewNotFound(autoscalingv1alpha1.Resource("Deciders"), key),
			createErr: want,
		})

	kpa := revisionresources.MakePA(newTestRevision(testNamespace, testRevision), nil)
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(ctx, kpa, metav1.CreateOptions{})
	fakepainformer.Get(ctx).Informer().GetIndexer().Add(kpa)

	sks := aresources.MakeSKS(kpa, nv1a1.SKSOperationModeServe, minActivators)
	sks.Status.PrivateServiceName = "bogus"
	fakenetworkingclient.Get(ctx).NetworkingV1alpha1().ServerlessServices(testNamespace).Create(ctx, sks, metav1.CreateOptions{})
	fakesksinformer.Get(ctx).Informer().GetIndexer().Add(sks)

	newDeployment(ctx, t, fakedynamicclient.Get(ctx), testRevision+"-deployment", 3)

	// The Reconciler won't do any work until it becomes the leader.
	if la, ok := ctl.Reconciler.(reconciler.LeaderAware); ok {
		la.Promote(reconciler.UniversalBucket(), func(reconciler.Bucket, types.NamespacedName) {})
	}

	got := ctl.Reconciler.Reconcile(ctx, key)
	if !errors.Is(got, want) {
		t.Errorf("Reconcile() = %v, wanted %v wrapped", got, want)
	}
}

func TestControllerUpdateError(t *testing.T) {
	ctx, cancel, infs := recTest.SetupFakeContextWithCancel(t, func(ctx context.Context) context.Context {
		return filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	})
	waitInformers, err := recTest.RunAndSyncInformers(ctx, infs...)
	if err != nil {
		t.Fatal("Error starting up informers:", err)
	}
	defer func() {
		cancel()
		waitInformers()
	}()

	want := apierrors.NewBadRequest("asdf")

	ctl := NewController(ctx, newConfigWatcher(),
		&failingDeciders{
			getErr:    apierrors.NewNotFound(autoscalingv1alpha1.Resource("Deciders"), key),
			createErr: want,
		})

	kpa := revisionresources.MakePA(newTestRevision(testNamespace, testRevision), nil)
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(ctx, kpa, metav1.CreateOptions{})
	fakepainformer.Get(ctx).Informer().GetIndexer().Add(kpa)

	sks := aresources.MakeSKS(kpa, nv1a1.SKSOperationModeServe, minActivators)
	sks.Status.PrivateServiceName = "bogus"
	fakenetworkingclient.Get(ctx).NetworkingV1alpha1().ServerlessServices(testNamespace).Create(ctx, sks, metav1.CreateOptions{})
	fakesksinformer.Get(ctx).Informer().GetIndexer().Add(sks)

	newDeployment(ctx, t, fakedynamicclient.Get(ctx), testRevision+"-deployment", 3)

	// The Reconciler won't do any work until it becomes the leader.
	if la, ok := ctl.Reconciler.(reconciler.LeaderAware); ok {
		la.Promote(reconciler.UniversalBucket(), func(reconciler.Bucket, types.NamespacedName) {})
	}

	got := ctl.Reconciler.Reconcile(ctx, key)
	if !errors.Is(got, want) {
		t.Errorf("Reconcile() = %v, wanted %v wrapped", got, want)
	}
}

func TestControllerGetError(t *testing.T) {
	ctx, cancel, infs := recTest.SetupFakeContextWithCancel(t, func(ctx context.Context) context.Context {
		return filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	})
	waitInformers, err := recTest.RunAndSyncInformers(ctx, infs...)
	if err != nil {
		t.Fatal("Error starting up informers:", err)
	}
	defer func() {
		cancel()
		waitInformers()
	}()

	want := apierrors.NewBadRequest("asdf")

	ctl := NewController(ctx, newConfigWatcher(),
		&failingDeciders{
			getErr: want,
		})

	kpa := revisionresources.MakePA(newTestRevision(testNamespace, testRevision), nil)
	fakeservingclient.Get(ctx).AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(ctx, kpa, metav1.CreateOptions{})
	fakepainformer.Get(ctx).Informer().GetIndexer().Add(kpa)

	sks := aresources.MakeSKS(kpa, nv1a1.SKSOperationModeServe, minActivators)
	sks.Status.PrivateServiceName = "bogus"
	fakenetworkingclient.Get(ctx).NetworkingV1alpha1().ServerlessServices(testNamespace).Create(ctx, sks, metav1.CreateOptions{})
	fakesksinformer.Get(ctx).Informer().GetIndexer().Add(sks)

	newDeployment(ctx, t, fakedynamicclient.Get(ctx), testRevision+"-deployment", 3)

	// The Reconciler won't do any work until it becomes the leader.
	if la, ok := ctl.Reconciler.(reconciler.LeaderAware); ok {
		la.Promote(reconciler.UniversalBucket(), func(reconciler.Bucket, types.NamespacedName) {})
	}

	got := ctl.Reconciler.Reconcile(ctx, key)
	if !errors.Is(got, want) {
		t.Errorf("Reconcile() = %v, wanted %v wrapped", got, want)
	}
}

func TestScaleFailure(t *testing.T) {
	ctx, cancel, infs := recTest.SetupFakeContextWithCancel(t, func(ctx context.Context) context.Context {
		return filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	})
	waitInformers, err := recTest.RunAndSyncInformers(ctx, infs...)
	if err != nil {
		t.Fatal("Error starting up informers:", err)
	}
	defer func() {
		cancel()
		waitInformers()
	}()

	ctl := NewController(ctx, newConfigWatcher(), newTestDeciders())

	// Only put the KPA in the lister, which will prompt failures scaling it.
	rev := newTestRevision(testNamespace, testRevision)
	kpa := revisionresources.MakePA(rev, nil)
	fakepainformer.Get(ctx).Informer().GetIndexer().Add(kpa)

	newDeployment(ctx, t, fakedynamicclient.Get(ctx), testRevision+"-deployment", 3)

	// The Reconciler won't do any work until it becomes the leader.
	if la, ok := ctl.Reconciler.(reconciler.LeaderAware); ok {
		la.Promote(reconciler.UniversalBucket(), func(reconciler.Bucket, types.NamespacedName) {})
	}

	if err := ctl.Reconciler.Reconcile(ctx, testNamespace+"/"+testRevision); err == nil {
		t.Error("Reconcile() = nil, wanted error")
	}
}

func pollDeciders(deciders *testDeciders, namespace, name string, cond func(*scaling.Decider) bool) (decider *scaling.Decider, err error) {
	wait.PollUntilContextTimeout(context.Background(), 10*time.Millisecond, 3*time.Second, true, func(context.Context) (bool, error) {
		decider, err = deciders.Get(context.Background(), namespace, name)
		if err != nil {
			return false, nil
		}
		return cond == nil || cond(decider), nil
	})
	return decider, err
}

func newTestDeciders() *testDeciders {
	return &testDeciders{
		createCallCount:    atomic.NewUint32(0),
		createCall:         make(chan struct{}, 1),
		deleteCallCount:    atomic.NewUint32(0),
		deleteCall:         make(chan struct{}, 5),
		updateCallCount:    atomic.NewUint32(0),
		updateCall:         make(chan struct{}, 1),
		deleteBeforeCreate: atomic.NewBool(false),
	}
}

type testDeciders struct {
	createCallCount *atomic.Uint32
	createCall      chan struct{}

	deleteCallCount *atomic.Uint32
	deleteCall      chan struct{}

	updateCallCount *atomic.Uint32
	updateCall      chan struct{}

	deleteBeforeCreate *atomic.Bool
	decider            *scaling.Decider
	mutex              sync.Mutex
}

func (km *testDeciders) Get(_ context.Context, namespace, name string) (*scaling.Decider, error) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	if km.decider == nil {
		return nil, apierrors.NewNotFound(autoscalingv1alpha1.Resource("Deciders"), types.NamespacedName{Namespace: namespace, Name: name}.String())
	}
	return km.decider, nil
}

func (km *testDeciders) Create(_ context.Context, decider *scaling.Decider) (*scaling.Decider, error) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	km.decider = decider
	km.createCallCount.Add(1)
	km.createCall <- struct{}{}
	return decider, nil
}

func (km *testDeciders) Delete(_ context.Context, _, _ string) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	km.decider = nil
	km.deleteCallCount.Add(1)
	if km.createCallCount.Load() == 0 {
		km.deleteBeforeCreate.Store(true)
	}
	km.deleteCall <- struct{}{}
}

func (km *testDeciders) Update(_ context.Context, decider *scaling.Decider) (*scaling.Decider, error) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	km.decider = decider
	km.updateCallCount.Add(1)
	km.updateCall <- struct{}{}
	return decider, nil
}

func (km *testDeciders) Watch(_ func(types.NamespacedName)) {}

type failingDeciders struct {
	getErr    error
	createErr error
}

func (km *failingDeciders) Get(_ context.Context, _, _ string) (*scaling.Decider, error) {
	return nil, km.getErr
}

func (km *failingDeciders) Create(_ context.Context, _ *scaling.Decider) (*scaling.Decider, error) {
	return nil, km.createErr
}

func (km *failingDeciders) Delete(_ context.Context, _, _ string) {}

func (km *failingDeciders) Watch(_ func(types.NamespacedName)) {
}

func (km *failingDeciders) Update(_ context.Context, decider *scaling.Decider) (*scaling.Decider, error) {
	return decider, nil
}

func newTestRevision(namespace, name string) *v1.Revision {
	return &v1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  fmt.Sprintf("/apis/ela/v1/namespaces/%s/revisions/%s", namespace, name),
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				autoscaling.ClassAnnotationKey: autoscaling.KPA,
			},
			Labels: map[string]string{
				serving.ServiceLabelKey:       "test-service",
				serving.ConfigurationLabelKey: "test-service",
			},
		},
		Spec: v1.RevisionSpec{},
	}
}

func makeReadyPods(num int, ns, n string) []runtime.Object {
	r := make([]runtime.Object, num)
	for i := 0; i < num; i++ {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      n + strconv.Itoa(i),
				Namespace: ns,
				Labels:    map[string]string{serving.RevisionLabelKey: n},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		}
		r[i] = p
	}
	return r
}

func withMinScale(minScale int) pkgTest.PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Annotations = kmeta.UnionMaps(
			pa.Annotations,
			map[string]string{autoscaling.MinScaleAnnotationKey: strconv.Itoa(minScale)},
		)
	}
}

func decider(ns, name string, desiredScale, ebc int32) *scaling.Decider {
	return &scaling.Decider{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Annotations: map[string]string{
				autoscaling.ClassAnnotationKey: autoscaling.KPA,
			},
		},
		Spec: scaling.DeciderSpec{
			MaxScaleUpRate:      10.0,
			TargetValue:         100,
			TotalValue:          100,
			TargetBurstCapacity: 211,
			PanicThreshold:      200,
			StableWindow:        60 * time.Second,
		},
		Status: scaling.DeciderStatus{
			DesiredScale:        desiredScale,
			ExcessBurstCapacity: ebc,
		},
	}
}

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

var _ reconciler.ConfigStore = (*testConfigStore)(nil)

func TestMetricsReporter(t *testing.T) {
	reader := otelmetric.NewManualReader()
	mp := otelmetric.NewMeterProvider(otelmetric.WithReader(reader))
	m := newMetrics(mp)

	pa := kpa(testNamespace, testRevision)

	expectedAttr := attribute.NewSet(
		metrics.ConfigurationNameKey.With(pa.Labels[serving.ConfigurationLabelKey]),
		metrics.K8sNamespaceKey.With(testNamespace),
		metrics.RevisionNameKey.With(testRevision),
		metrics.ServiceNameKey.With(pa.Labels[serving.ServiceLabelKey]),
	)

	pc := podCounts{
		want:        1982,
		ready:       1984,
		notReady:    1988,
		pending:     1996,
		terminating: 1983,
	}
	reportMetrics(m, pa, pc)

	metricstest.AssertMetrics(t, reader,
		metricstest.MetricsEqual(
			scopeName,
			metricdata.Metrics{
				Name:        "kn.revision.pods.requested",
				Description: "Number of pods autoscaler requested from Kubernetes",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1982,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.count",
				Description: "Number of pods that are allocated currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1984,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.not_ready.count",
				Description: "Number of pods that are not ready currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1988,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.pending.count",
				Description: "Number of pods that are pending currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1996,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.terminating.count",
				Description: "Number of pods that are terminating currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1983,
						Attributes: expectedAttr,
					}},
				},
			},
		),
	)

	// Verify `want` is ignored, when it is equal to -1.
	pc.want = -1
	pc.terminating = 1955
	reportMetrics(m, pa, pc)

	metricstest.AssertMetrics(t, reader,
		metricstest.MetricsEqual(
			scopeName,
			metricdata.Metrics{
				Name:        "kn.revision.pods.requested",
				Description: "Number of pods autoscaler requested from Kubernetes",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1982,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.count",
				Description: "Number of pods that are allocated currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1984,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.not_ready.count",
				Description: "Number of pods that are not ready currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1988,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.pending.count",
				Description: "Number of pods that are pending currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1996,
						Attributes: expectedAttr,
					}},
				},
			},
			metricdata.Metrics{
				Name:        "kn.revision.pods.terminating.count",
				Description: "Number of pods that are terminating currently",
				Unit:        "{pod}",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{{
						Value:      1955,
						Attributes: expectedAttr,
					}},
				},
			},
		),
	)
}

func TestResolveScrapeTarget(t *testing.T) {
	pa := kpa(testNamespace, testRevision, pkgTest.WithPAMetricsService("echo"))
	tc := &testConfigStore{config: defaultConfig()}

	if got, want := resolveScrapeTarget(tc.ToContext(context.Background()), pa), "echo"; got != want {
		t.Errorf("reconcileMetricSN()= %s, want %s", got, want)
	}

	tc.config.Autoscaler.TargetBurstCapacity = -1
	if got, want := resolveScrapeTarget(tc.ToContext(context.Background()), pa), ""; got != want {
		t.Errorf("reconcileMetricSN()= %s, want %s", got, want)
	}

	tc = &testConfigStore{config: defaultConfig()}
	pa.Annotations[autoscaling.TargetBurstCapacityKey] = "-1"
	if got, want := resolveScrapeTarget(tc.ToContext(context.Background()), pa), ""; got != want {
		t.Errorf("reconcileMetricSN()= %s, want %s", got, want)
	}
}

func withInitialScale() pkgTest.PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Annotations = kmeta.UnionMaps(
			pa.Annotations,
			map[string]string{autoscaling.InitialScaleAnnotationKey: strconv.Itoa(20)},
		)
	}
}

func TestComputeActivatorNum(t *testing.T) {
	cases := []struct {
		name    string
		pods    int
		decider *scaling.Decider
		want    int32
	}{{
		name: "default",
		pods: 0,
		decider: &scaling.Decider{
			Spec: scaling.DeciderSpec{
				TotalValue:          100,
				TargetBurstCapacity: 200,
				ActivatorCapacity:   100,
			},
		},
		want: minActivators,
	}, {
		name: "1 pod",
		pods: 1,
		decider: &scaling.Decider{
			Spec: scaling.DeciderSpec{
				TotalValue:          100,
				TargetBurstCapacity: 200,
				ActivatorCapacity:   100,
			},
		},
		want: 3,
	}, {
		name: "tbc = 0",
		pods: 100,
		decider: &scaling.Decider{
			Spec: scaling.DeciderSpec{
				TotalValue:          100,
				TargetBurstCapacity: 0,
				ActivatorCapacity:   100,
			},
		},
		want: minActivators,
	}, {
		name: "tbc = -1",
		pods: 5,
		decider: &scaling.Decider{
			Spec: scaling.DeciderSpec{
				TotalValue:          100,
				TargetBurstCapacity: -1,
				ActivatorCapacity:   100,
			},
		},
		want: 5,
	}, {
		name: "many pods",
		pods: 15,
		decider: &scaling.Decider{
			Spec: scaling.DeciderSpec{
				TotalValue:          100,
				TargetBurstCapacity: 200,
				ActivatorCapacity:   100,
			},
		},
		want: 17,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeNumActivators(c.pods, c.decider)
			if got != c.want {
				t.Errorf("computeNumActivators() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestComputeStatus(t *testing.T) {
	cases := []struct {
		name string

		haveActual       *int32
		haveDesiredScale *int32

		pcReady int
		pcWant  int

		wantActualScale  *int32
		wantDesiredScale *int32
	}{{
		name: "initial",

		haveActual:       nil,
		haveDesiredScale: nil,

		pcReady: 0,
		pcWant:  1,

		wantActualScale:  ptr.Int32(0),
		wantDesiredScale: ptr.Int32(1),
	}, {
		name: "ready",

		haveActual:       ptr.Int32(0),
		haveDesiredScale: ptr.Int32(1),

		pcReady: 1,
		pcWant:  1,

		wantActualScale:  ptr.Int32(1),
		wantDesiredScale: ptr.Int32(1),
	}, {
		name: "stable",

		haveActual:       ptr.Int32(1),
		haveDesiredScale: ptr.Int32(1),

		pcReady: 1,
		pcWant:  1,

		wantActualScale:  ptr.Int32(1),
		wantDesiredScale: ptr.Int32(1),
	}, {
		name: "no metrics",

		haveActual:       ptr.Int32(1),
		haveDesiredScale: ptr.Int32(2),

		pcReady: 2,
		pcWant:  -1,

		wantActualScale:  ptr.Int32(2),
		wantDesiredScale: ptr.Int32(2),
	}}

	tc := &testConfigStore{config: defaultConfig()}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := tc.ToContext(context.Background())

			pa := &autoscalingv1alpha1.PodAutoscaler{
				Status: autoscalingv1alpha1.PodAutoscalerStatus{
					ActualScale:  c.haveActual,
					DesiredScale: c.haveDesiredScale,
				},
			}
			pc := podCounts{
				ready: c.pcReady,
				want:  c.pcWant,
			}

			computeStatus(ctx, pa, nil, pc, logging.FromContext(ctx), nil)

			if c.wantActualScale == nil && pa.Status.ActualScale != nil || c.wantActualScale != nil && pa.Status.ActualScale == nil {
				t.Errorf("Unexpected ActualScale. Want: %v, Got: %v", c.wantActualScale, pa.Status.ActualScale)
			}
			if c.wantActualScale != nil && pa.Status.ActualScale != nil && *c.wantActualScale != *pa.Status.ActualScale {
				t.Errorf("Unexpected ActualScale. Want: %d, Got: %d", *c.wantActualScale, *pa.Status.ActualScale)
			}

			if c.wantDesiredScale == nil && pa.Status.DesiredScale != nil || c.wantDesiredScale != nil && pa.Status.DesiredScale == nil {
				t.Errorf("Unexpected DesiredScale. Want: %v, Got: %v", c.wantDesiredScale, pa.Status.DesiredScale)
			}
			if c.wantDesiredScale != nil && pa.Status.DesiredScale != nil && *c.wantDesiredScale != *pa.Status.DesiredScale {
				t.Errorf("Unexpected DesiredScale. Want: %d, Got: %d", *c.wantDesiredScale, *pa.Status.DesiredScale)
			}
		})
	}
}

func TestReconcileWithNSR(t *testing.T) {
	testRevision := "testRevision1"
	testNamespace := "testNamespace1"
	key := testNamespace + "/" + testRevision
	deployName := testRevision + "-deployment"
	const (
		defaultScale = 19
		defaultAct   = 3 // 1-10 ready pods + 200 TBC
	)
	privateSvc := names.PrivateService(testRevision)

	// Set up a default deployment with the appropriate scale so that we don't
	// see patches to correct that scale.
	defaultDeployment := deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
		d.Spec.Replicas = ptr.Int32(defaultScale)
	})

	defaultSKS := sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithSKSReady, pkgTest.WithNumActivators(defaultAct))

	defaultReady := makeReadyPods(1, testNamespace, testRevision)[0]

	type deciderKey struct{}
	type asConfigKey struct{}
	type netConfigKey struct{}

	// Note: due to how KPA reconciler works we are dependent on the
	// two constant objects above, which means, that all tests must share
	// the same namespace and revision name.
	table := recTest.TableTest{{
		Name: "bad workqueue key, Part I",
		Key:  "too/many/parts",
	}, {
		Name: "bad workqueue key, Part II",
		Key:  "too-few-parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}, {
		Name: "steady state",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithPASKSReady, pkgTest.WithTraffic,
				markScaleTargetInitialized, pkgTest.WithPAMetricsService(privateSvc),
				withScales(1, defaultScale), pkgTest.WithPAStatusService(testRevision), pkgTest.WithObservedGeneration(1)),
			defaultSKS,
			metric(testNamespace, testRevision),
			defaultDeployment, defaultReady},
	}, {
		Name: "steady not serving",
		Key:  key,
		Ctx: context.WithValue(context.Background(), deciderKey{},
			decider(testNamespace, testRevision, 0 /* desiredScale */, 0 /* ebc */)),
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, pkgTest.WithScaleTargetInitialized, withScales(0, 0),
				pkgTest.WithNoTraffic(noTrafficReason, "The target is not receiving traffic."),
				pkgTest.WithPASKSReady, markOld, pkgTest.WithPAStatusService(testRevision),
				pkgTest.WithPAMetricsService(privateSvc), pkgTest.WithObservedGeneration(1)),
			sks(testNamespace, testRevision, pkgTest.WithDeployRef(deployName), pkgTest.WithProxyMode, pkgTest.WithSKSReady),
			metric(testNamespace, testRevision),
			deploy(testNamespace, testRevision, func(d *appsv1.Deployment) {
				d.Spec.Replicas = ptr.Int32(0)
			}),
		},
	}}
	table.Test(t, testV1.MakeFactory(func(ctx context.Context, listers *testV1.Listers, _ configmap.Watcher) controller.Reconciler {
		ctx = podscalable.WithDuck(ctx)

		fakeDeciders := newTestDeciders()
		// TODO(vagababov): see if we can get rid of the static piece of configuration and
		// constant namespace and revision names.

		// Make new decider if it's not in the context
		if d := ctx.Value(deciderKey{}); d == nil {
			decider := resources.MakeDecider(
				kpa(testNamespace, testRevision), defaultConfig().Autoscaler)
			decider.Status.DesiredScale = defaultScale
			decider.Generation = 2112
			fakeDeciders.Create(ctx, decider)
		} else {
			fakeDeciders.Create(ctx, d.(*scaling.Decider))
		}

		testConfigs := defaultConfig()
		if asConfig := ctx.Value(asConfigKey{}); asConfig != nil {
			testConfigs.Autoscaler = asConfig.(*autoscalerconfig.Config)
		}
		if netConfig := ctx.Value(netConfigKey{}); netConfig != nil {
			testConfigs.Network = netConfig.(*netcfg.Config)
		}
		psf := podscalable.Get(ctx)
		scaler := newScaler(ctx, psf, func(interface{}, time.Duration) {})
		scaler.activatorProbe = func(*autoscalingv1alpha1.PodAutoscaler, http.RoundTripper) (bool, error) { return true, nil }
		r := &Reconciler{
			Base: &areconciler.Base{
				Client:           servingclient.Get(ctx),
				NetworkingClient: networkingclient.Get(ctx),
				SKSLister:        listers.GetServerlessServiceLister(),
				MetricLister:     listers.GetMetricLister(),
			},
			podsLister: listers.GetPodsLister(),
			deciders:   fakeDeciders,
			scaler:     scaler,
			spaLister:  listers.GetStagePodAutoscalerLister(),
		}
		return pareconciler.NewReconciler(ctx, logging.FromContext(ctx),
			servingclient.Get(ctx), listers.GetPodAutoscalerLister(),
			controller.GetEventRecorder(ctx), r, autoscaling.KPA,
			controller.Options{
				ConfigStore: &testConfigStore{config: testConfigs},
			})
	}))
}
