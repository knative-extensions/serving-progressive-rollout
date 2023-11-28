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

package rolloutorchestrator

import (
	"context"

	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/logging"

	"knative.dev/pkg/controller"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	servingclient "knative.dev/serving-progressive-rollout/pkg/client/injection/client"
	roinformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/rolloutorchestrator"
	spainformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/stagepodautoscaler"
	roreconciler "knative.dev/serving-progressive-rollout/pkg/client/injection/reconciler/serving/v1/rolloutorchestrator"
	cfgmap "knative.dev/serving/pkg/apis/config"
)

// NewController creates a new rolloutorchestrator controller
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	soInformer := roinformer.Get(ctx)
	stagePodAutoscalerInformer := spainformer.Get(ctx)

	configStore := cfgmap.NewStore(logger.Named("config-store"))
	configStore.WatchConfigs(cmw)

	c := &Reconciler{
		client:                   servingclient.Get(ctx),
		stagePodAutoscalerLister: stagePodAutoscalerInformer.Lister(),
	}
	opts := func(*controller.Impl) controller.Options {
		return controller.Options{ConfigStore: configStore}
	}
	impl := roreconciler.NewImpl(ctx, c, opts)

	soInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	handleMatchingControllers := cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterController(&v1.RolloutOrchestrator{}),
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	}
	stagePodAutoscalerInformer.Informer().AddEventHandler(handleMatchingControllers)

	return impl
}
