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
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	deploymentinformer "knative.dev/pkg/client/injection/kube/informers/apps/v1/deployment"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	servingclient "knative.dev/serving-progressive-rollout/pkg/client/injection/client"
	roinformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/rolloutorchestrator"
	spainformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/stagepodautoscaler"
	roreconciler "knative.dev/serving-progressive-rollout/pkg/client/injection/reconciler/serving/v1/rolloutorchestrator"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/common"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/rolloutorchestrator/rolloutmodes"
	cfgmap "knative.dev/serving/pkg/apis/config"
	revisioninformer "knative.dev/serving/pkg/client/injection/informers/serving/v1/revision"
)

// NewController creates a new RolloutOrchestrator controller
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	roInformer := roinformer.Get(ctx)
	stagePodAutoscalerInformer := spainformer.Get(ctx)
	deploymentInformer := deploymentinformer.Get(ctx)
	revisionInformer := revisioninformer.Get(ctx)

	configStore := cfgmap.NewStore(logger.Named(common.ConfigStoreName))
	configStore.WatchConfigs(cmw)

	rolloutMode := rolloutmodes.NewRolloutModes(servingclient.Get(ctx), kubeclient.Get(ctx), stagePodAutoscalerInformer.Lister())
	c := &Reconciler{
		client:                   servingclient.Get(ctx),
		stagePodAutoscalerLister: stagePodAutoscalerInformer.Lister(),
		deploymentLister:         deploymentInformer.Lister(),
		revisionLister:           revisionInformer.Lister(),
		rolloutMode:              rolloutMode,
	}

	opts := func(*controller.Impl) controller.Options {
		return controller.Options{ConfigStore: configStore}
	}

	// Create a controller.Impl that handles queuing and feeding work from
	// the queue through an implementation of controller.Reconciler for the RolloutOrchestrator.
	impl := roreconciler.NewImpl(ctx, c, opts)
	c.enqueueAfter = impl.EnqueueAfter

	// This reconciliation loop of the RolloutOrchestrator will watch the changes of RolloutOrchestrator itself.
	roInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	handleMatchingControllers := cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterController(&v1.RolloutOrchestrator{}),
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	}
	// Since RolloutOrchestrator owns the StagePodAutoscaler, this reconciliation loop of the
	// RolloutOrchestrator will watch the changes of stagePodAutoscaler, as the child.
	stagePodAutoscalerInformer.Informer().AddEventHandler(handleMatchingControllers)

	return impl
}
