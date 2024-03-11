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

package stagepodautoscaler

import (
	"context"

	"k8s.io/client-go/tools/cache"
	filteredpodinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod/filtered"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	spainformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/stagepodautoscaler"
	spareconciler "knative.dev/serving-progressive-rollout/pkg/client/injection/reconciler/serving/v1/stagepodautoscaler"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/common"
	cfgmap "knative.dev/serving/pkg/apis/config"
	"knative.dev/serving/pkg/apis/serving"
	servingclient "knative.dev/serving/pkg/client/injection/client"
	painformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/podautoscaler"
)

// NewController creates a new StagePodAutoscaler controller
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,

) *controller.Impl {
	logger := logging.FromContext(ctx)
	paInformer := painformer.Get(ctx)
	stagePodAutoscalerInformer := spainformer.Get(ctx)
	podsInformer := filteredpodinformer.Get(ctx, serving.RevisionUID)

	configStore := cfgmap.NewStore(logger.Named(common.ConfigStoreName))
	configStore.WatchConfigs(cmw)

	c := &Reconciler{
		client:              servingclient.Get(ctx),
		podAutoscalerLister: paInformer.Lister(),
		podsLister:          podsInformer.Lister(),
	}
	opts := func(*controller.Impl) controller.Options {
		return controller.Options{ConfigStore: configStore}
	}

	// Creat a controller.Impl that handles queuing and feeding work from
	// the queue through an implementation of controller.Reconciler for the StagePodAutoscaler.
	impl := spareconciler.NewImpl(ctx, c, opts)

	// This reconciliation loop of the StagePodAutoscaler will watch the changes of stagePodAutoscaler itself.
	stagePodAutoscalerInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	podsInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: pkgreconciler.LabelExistsFilterFunc(serving.RevisionLabelKey),
		Handler:    controller.HandleAll(impl.EnqueueLabelOfNamespaceScopedResource("", serving.RevisionLabelKey)),
	})

	// Create a FilteringResourceEventHandler, that will match the PodAutoscaler having the same
	// value for the key serving.RevisionLabelKey as the StagePodAutoscaler. The purpose is to make sure
	// StagePodAutoscaler's reconcile loop can watch the changes in the PodAutoscaler.

	// The value for the key serving.RevisionLabelKey is set to the name of the revision.

	// PodAutoscaler has a one-one-one mapping relation with the revision, sharing the same name.
	// StagePodAutoscaler also has a one-one-one mapping relation with the revision, sharing the same name.
	// PodAutoscaler and StagePodAutoscaler for the same revision share the same name and the same value for
	// the key serving.RevisionLabelKey.
	handleMatchingController := cache.FilteringResourceEventHandler{
		FilterFunc: pkgreconciler.LabelExistsFilterFunc(serving.RevisionLabelKey),
		Handler: controller.HandleAll(impl.EnqueueLabelOfNamespaceScopedResource("",
			serving.RevisionLabelKey)),
	}
	paInformer.Informer().AddEventHandler(handleMatchingController)
	return impl
}
