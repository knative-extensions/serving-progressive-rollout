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

package service

import (
	"context"

	"k8s.io/client-go/tools/cache"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/serving-progressive-rollout/pkg/client/injection/client"
	roinformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/rolloutorchestrator"
	rolloutorchestratorinformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/rolloutorchestrator"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/common"
	cfgmap "knative.dev/serving/pkg/apis/config"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	servingclient "knative.dev/serving/pkg/client/injection/client"
	painformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/podautoscaler"
	configurationinformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/configuration"
	revisioninformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/revision"
	routeinformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/route"
	kserviceinformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/service"
	ksvcreconciler "knative.dev/serving/pkg/client/injection/reconciler/serving/v1/service"
)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	serviceInformer := kserviceinformer.Get(ctx)
	routeInformer := routeinformer.Get(ctx)
	configurationInformer := configurationinformer.Get(ctx)
	revisionInformer := revisioninformer.Get(ctx)
	rolloutorchestratorInformer := rolloutorchestratorinformer.Get(ctx)
	paInformer := painformer.Get(ctx)
	roInformer := roinformer.Get(ctx)

	configStore := cfgmap.NewStore(logger.Named(common.ConfigStoreName))
	configStore.WatchConfigs(cmw)

	c := NewReconciler(
		client.Get(ctx),
		servingclient.Get(ctx),
		configurationInformer.Lister(),
		revisionInformer.Lister(),
		routeInformer.Lister(),
		rolloutorchestratorInformer.Lister(),
		paInformer.Lister(),
	)

	opts := func(*controller.Impl) controller.Options {
		return controller.Options{ConfigStore: configStore}
	}
	impl := ksvcreconciler.NewImpl(ctx, c, opts)
	c.enqueueAfter = impl.EnqueueAfter

	serviceInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	handleControllerOf := cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterController(&v1.Service{}),
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	}
	configurationInformer.Informer().AddEventHandler(handleControllerOf)
	routeInformer.Informer().AddEventHandler(handleControllerOf)

	// The reconciliation loop of the service listens to the changes on the CR RolloutOchestrator.
	roInformer.Informer().AddEventHandler(handleControllerOf)

	return impl
}
