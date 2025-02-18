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

// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	context "context"
	time "time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
	apisservingv1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	versioned "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	internalinterfaces "knative.dev/serving-progressive-rollout/pkg/client/informers/externalversions/internalinterfaces"
	servingv1 "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
)

// StagePodAutoscalerInformer provides access to a shared informer and lister for
// StagePodAutoscalers.
type StagePodAutoscalerInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() servingv1.StagePodAutoscalerLister
}

type stagePodAutoscalerInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewStagePodAutoscalerInformer constructs a new informer for StagePodAutoscaler type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewStagePodAutoscalerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredStagePodAutoscalerInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredStagePodAutoscalerInformer constructs a new informer for StagePodAutoscaler type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredStagePodAutoscalerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ServingV1().StagePodAutoscalers(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ServingV1().StagePodAutoscalers(namespace).Watch(context.TODO(), options)
			},
		},
		&apisservingv1.StagePodAutoscaler{},
		resyncPeriod,
		indexers,
	)
}

func (f *stagePodAutoscalerInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredStagePodAutoscalerInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *stagePodAutoscalerInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&apisservingv1.StagePodAutoscaler{}, f.defaultInformer)
}

func (f *stagePodAutoscalerInformer) Lister() servingv1.StagePodAutoscalerLister {
	return servingv1.NewStagePodAutoscalerLister(f.Informer().GetIndexer())
}
