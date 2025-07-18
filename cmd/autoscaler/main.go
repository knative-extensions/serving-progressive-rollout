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

// Multitenant autoscaler executable.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/stagepodautoscaler"

	netcfg "knative.dev/networking/pkg/config"
	filteredpodinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod/filtered"
	filteredinformerfactory "knative.dev/pkg/client/injection/kube/informers/factory/filtered"
	configmap "knative.dev/pkg/configmap/informer"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/leaderelection"
	"knative.dev/pkg/logging"
	k8sruntime "knative.dev/pkg/observability/runtime/k8s"
	"knative.dev/pkg/signals"
	"knative.dev/pkg/system"
	"knative.dev/pkg/version"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/autoscaling/kpa"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/apis/serving"
	"knative.dev/serving/pkg/autoscaler/bucket"
	asmetrics "knative.dev/serving/pkg/autoscaler/metrics"
	"knative.dev/serving/pkg/autoscaler/scaling"
	"knative.dev/serving/pkg/autoscaler/statforwarder"
	"knative.dev/serving/pkg/autoscaler/statserver"
	smetrics "knative.dev/serving/pkg/metrics"
	o11yconfigmap "knative.dev/serving/pkg/observability/configmap"
	"knative.dev/serving/pkg/observability/otel"
	servingmetric "knative.dev/serving/pkg/reconciler/metric"
	"knative.dev/serving/pkg/resources"
)

const (
	statsServerAddr = ":8080"
	statsBufferLen  = 1000
	component       = "autoscaler"
	controllerNum   = 2
)

func main() {
	// Set up signals so we handle the first shutdown signal gracefully.
	ctx := signals.NewContext()

	cfg := injection.ParseAndGetRESTConfigOrDie()

	log.Printf("Registering %d clients", len(injection.Default.GetClients()))
	log.Printf("Registering %d informer factories", len(injection.Default.GetInformerFactories()))
	log.Printf("Registering %d informers", len(injection.Default.GetInformers()))
	log.Printf("Registering %d filtered informers", len(injection.Default.GetFilteredInformers()))
	log.Printf("Registering %d controllers", controllerNum)

	// Adjust our client's rate limits based on the number of controller's we are running.
	cfg.QPS = controllerNum * rest.DefaultQPS
	cfg.Burst = controllerNum * rest.DefaultBurst
	ctx = filteredinformerfactory.WithSelectors(ctx, serving.RevisionUID)
	ctx, informers := injection.Default.SetupInformers(ctx, cfg)

	kubeClient := kubeclient.Get(ctx)

	// We sometimes startup faster than we can reach kube-api. Poll on failure to prevent us terminating
	var err error
	if perr := wait.PollUntilContextTimeout(ctx, time.Second, 60*time.Second, true, func(context.Context) (bool, error) {
		if err = version.CheckMinimumVersion(kubeClient.Discovery()); err != nil {
			log.Print("Failed to get k8s version ", err)
		}
		return err == nil, nil
	}); perr != nil {
		log.Fatal("Timed out attempting to get k8s version: ", err)
	}

	// Set up our logger.
	loggingConfig, err := sharedmain.GetLoggingConfig(ctx)
	if err != nil {
		log.Fatal("Error loading/parsing logging configuration: ", err)
	}
	logger, atomicLevel := logging.NewLoggerFromConfig(loggingConfig, component)
	defer logger.Sync()
	ctx = logging.WithLogger(ctx, logger)

	pprof := k8sruntime.NewProfilingServer(logger.Named("pprof"))
	mp, tp := otel.SetupObservabilityOrDie(ctx, "autoscaler", logger, pprof)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mp.Shutdown(ctx); err != nil {
			logger.Errorw("Error flushing metrics", zap.Error(err))
		}

		if err := tp.Shutdown(ctx); err != nil {
			logger.Errorw("Error flushing traces", zap.Error(err))
		}
	}()

	// statsCh is the main communication channel between the stats server and multiscaler.
	statsCh := make(chan asmetrics.StatMessage, statsBufferLen)
	defer close(statsCh)

	cmw := configmap.NewInformedWatcher(kubeclient.Get(ctx), system.Namespace())
	// Watch the logging config map and dynamically update logging levels.
	cmw.Watch(logging.ConfigMapName(), logging.UpdateLevelFromConfigMap(logger, atomicLevel, component))
	// Watch the observability config map
	cmw.Watch(o11yconfigmap.Name(), pprof.UpdateFromConfigMap)

	podLister := filteredpodinformer.Get(ctx, serving.RevisionUID).Lister()
	networkCM, err := kubeclient.Get(ctx).CoreV1().ConfigMaps(system.Namespace()).Get(ctx, netcfg.ConfigMapName, metav1.GetOptions{})
	if err != nil {
		logger.Fatalw("Failed to fetch network config", zap.Error(err))
	}
	networkConfig, err := netcfg.NewConfigFromMap(networkCM.Data)
	if err != nil {
		logger.Fatalw("Failed to construct network config", zap.Error(err))
	}

	collector := asmetrics.NewMetricCollector(
		statsScraperFactoryFunc(podLister, networkConfig.EnableMeshPodAddressability, networkConfig.MeshCompatibilityMode, mp), logger)

	// Set up scalers.
	multiScaler := scaling.NewMultiScaler(ctx.Done(),
		uniScalerFactoryFunc(mp, podLister, collector), logger)

	controllers := []*controller.Impl{
		kpa.NewController(ctx, cmw, multiScaler),
		servingmetric.NewController(ctx, cmw, collector),
		stagepodautoscaler.NewController(ctx, cmw),
	}

	// Start watching the configs.
	if err := cmw.Start(ctx.Done()); err != nil {
		logger.Fatalw("Failed to start watching configs", zap.Error(err))
	}

	// Start all of the informers and wait for them to sync.
	if err := controller.StartInformers(ctx.Done(), informers...); err != nil {
		logger.Fatalw("Failed to start informers", zap.Error(err))
	}

	// accept is the func to call when this pod owns the Revision for this StatMessage.
	accept := func(sm asmetrics.StatMessage) {
		collector.Record(sm.Key, time.Unix(sm.Stat.Timestamp, 0), sm.Stat)
		multiScaler.Poke(sm.Key, sm.Stat)
	}

	cc := componentConfigAndIP(ctx)

	// We don't want an elector on the controller context
	// since they will be sharing an elector
	var electorCtx context.Context

	var f *statforwarder.Forwarder
	if b, bs, err := leaderelection.NewStatefulSetBucketAndSet(int(cc.Buckets)); err == nil {
		logger.Info("Running with StatefulSet leader election")
		electorCtx = leaderelection.WithStatefulSetElectorBuilder(ctx, cc, b)
		f = statforwarder.New(ctx, bs)
		if err := statforwarder.StatefulSetBasedProcessor(ctx, f, accept); err != nil {
			logger.Fatalw("Failed to set up statefulset processors", zap.Error(err))
		}
	} else {
		logger.Info("Running with Standard leader election")
		electorCtx = leaderelection.WithStandardLeaderElectorBuilder(ctx, kubeClient, cc)
		f = statforwarder.New(ctx, bucket.AutoscalerBucketSet(cc.Buckets))
		if err := statforwarder.LeaseBasedProcessor(ctx, f, accept); err != nil {
			logger.Fatalw("Failed to set up lease tracking", zap.Error(err))
		}
	}

	elector, err := setupSharedElector(electorCtx, controllers)
	if err != nil {
		logger.Fatalw("Failed to setup elector", zap.Error(err))
	}

	// Set up a statserver.
	statsServer := statserver.New(statsServerAddr, statsCh, logger, f.IsBucketOwner)
	defer f.Cancel()

	go func() {
		for sm := range statsCh {
			// Set the timestamp when first receiving the stat.
			if sm.Stat.Timestamp == 0 {
				sm.Stat.Timestamp = time.Now().Unix()
			}
			f.Process(sm)
		}
	}()

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		elector.Run(egCtx)
		return nil
	})
	eg.Go(statsServer.ListenAndServe)
	eg.Go(pprof.ListenAndServe)
	eg.Go(func() error {
		return controller.StartAll(egCtx, controllers...)
	})

	// This will block until either a signal arrives or one of the grouped functions
	// returns an error.
	<-egCtx.Done()

	statsServer.Shutdown(5 * time.Second)
	pprof.Shutdown(context.Background())
	// Don't forward ErrServerClosed as that indicates we're already shutting down.
	if err := eg.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Errorw("Error while running server", zap.Error(err))
	}
}

func uniScalerFactoryFunc(
	mp metric.MeterProvider,
	podLister corev1listers.PodLister,
	metricClient asmetrics.MetricClient,
) scaling.UniScalerFactory {
	return func(decider *scaling.Decider) (scaling.UniScaler, error) {
		configName := decider.Labels[serving.ConfigurationLabelKey]
		if configName == "" {
			return nil, fmt.Errorf("label %q not found or empty in Decider %s", serving.ConfigurationLabelKey, decider.Name)
		}
		revisionName := decider.Labels[serving.RevisionLabelKey]
		if revisionName == "" {
			return nil, fmt.Errorf("label %q not found or empty in Decider %s", serving.RevisionLabelKey, decider.Name)
		}
		serviceName := decider.Labels[serving.ServiceLabelKey] // This can be empty.

		// Create a stats reporter which tags statistics by PA namespace, configuration name, and PA name.
		attrs := attribute.NewSet(
			smetrics.ConfigurationNameKey.With(configName),
			smetrics.RevisionNameKey.With(revisionName),
			smetrics.ServiceNameKey.With(serviceName),
			smetrics.K8sNamespaceKey.With(decider.Namespace),
		)

		podAccessor := resources.NewPodAccessor(podLister, decider.Namespace, revisionName)

		return scaling.New(
			attrs,
			mp,
			decider.Namespace,
			decider.Name,
			metricClient,
			podAccessor,
			&decider.Spec), nil
	}
}

func statsScraperFactoryFunc(podLister corev1listers.PodLister, usePassthroughLb bool, meshMode netcfg.MeshCompatibilityMode, mp metric.MeterProvider) asmetrics.StatsScraperFactory {
	return func(metric *autoscalingv1alpha1.Metric, logger *zap.SugaredLogger) (asmetrics.StatsScraper, error) {
		if metric.Spec.ScrapeTarget == "" {
			return nil, nil
		}

		revisionName := metric.Labels[serving.RevisionLabelKey]
		if revisionName == "" {
			return nil, fmt.Errorf("label %q not found or empty in Metric %s", serving.RevisionLabelKey, metric.Name)
		}

		podAccessor := resources.NewPodAccessor(podLister, metric.Namespace, revisionName)
		return asmetrics.NewStatsScraper(metric, revisionName, podAccessor, usePassthroughLb, meshMode, logger, mp), nil
	}
}

func componentConfigAndIP(ctx context.Context) leaderelection.ComponentConfig {
	id, err := bucket.Identity()
	if err != nil {
		logging.FromContext(ctx).Fatalw("Failed to generate Lease holder identity", zap.Error(err))
	}

	// Set up leader election config
	leaderElectionConfig, err := sharedmain.GetLeaderElectionConfig(ctx)
	if err != nil {
		logging.FromContext(ctx).Fatalw("Error loading leader election configuration", zap.Error(err))
	}

	cc := leaderElectionConfig.GetComponentConfig(component)
	cc.LeaseName = func(i uint32) string {
		return bucket.AutoscalerBucketName(i, cc.Buckets)
	}
	cc.Identity = id

	return cc
}
