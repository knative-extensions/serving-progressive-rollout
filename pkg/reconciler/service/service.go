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

	pkgreconciler "knative.dev/pkg/reconciler"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	clientset "knative.dev/serving/pkg/client/clientset/versioned"
	ksvcreconciler "knative.dev/serving/pkg/client/injection/reconciler/serving/v1/service"
	listers "knative.dev/serving/pkg/client/listers/serving/v1"
	servingService "knative.dev/serving/pkg/reconciler/service"
)

// Reconciler implements controller.Reconciler for Service resources.
type Reconciler struct {
	baseReconciler *servingService.Reconciler
}

// Check that our Reconciler implements ksvcreconciler.Interface
var _ ksvcreconciler.Interface = (*Reconciler)(nil)

// NewReconciler creates the reference to the Reconciler based on clientset.Interface, listers.ConfigurationLister,
// listers.RevisionLister and listers.RouteLister.
func NewReconciler(client clientset.Interface, configurationLister listers.ConfigurationLister,
	revisionLister listers.RevisionLister, routeLister listers.RouteLister) *Reconciler {
	return &Reconciler{
		baseReconciler: servingService.NewReconciler(
			client,
			configurationLister,
			revisionLister,
			routeLister,
		),
	}
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, service *v1.Service) pkgreconciler.Event {
	return c.baseReconciler.ReconcileKind(ctx, service)
}
