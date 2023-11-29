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

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	spainformer "knative.dev/serving-progressive-rollout/pkg/client/injection/informers/serving/v1/stagepodautoscaler"
	spareconciler "knative.dev/serving-progressive-rollout/pkg/client/injection/reconciler/serving/v1/stagepodautoscaler"
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

	configStore := cfgmap.NewStore(logger.Named("config-store"))
	configStore.WatchConfigs(cmw)

	c := &Reconciler{
		client:              servingclient.Get(ctx),
		podAutoscalerLister: paInformer.Lister(),
	}
	opts := func(*controller.Impl) controller.Options {
		return controller.Options{ConfigStore: configStore}
	}
	impl := spareconciler.NewImpl(ctx, c, opts)

	stagePodAutoscalerInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	handleMatchingControllers := cache.FilteringResourceEventHandler{
		FilterFunc: pkgreconciler.LabelExistsFilterFunc(serving.RevisionLabelKey),
		Handler: controller.HandleAll(impl.EnqueueLabelOfNamespaceScopedResource("",
			serving.RevisionLabelKey)),
	}
	paInformer.Informer().AddEventHandler(handleMatchingControllers)
	return impl
}
