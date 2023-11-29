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
	"fmt"

	apierrs "k8s.io/apimachinery/pkg/api/errors"

	pkgreconciler "knative.dev/pkg/reconciler"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	spareconciler "knative.dev/serving-progressive-rollout/pkg/client/injection/reconciler/serving/v1/stagepodautoscaler"
	clientset "knative.dev/serving/pkg/client/clientset/versioned"
	palisters "knative.dev/serving/pkg/client/listers/autoscaling/v1alpha1"
)

// Reconciler implements controller.Reconciler for StagePodAutoscaler resources.
type Reconciler struct {
	client              clientset.Interface
	podAutoscalerLister palisters.PodAutoscalerLister
}

// Check that our Reconciler implements soreconciler.Interface
var _ spareconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, spa *v1.StagePodAutoscaler) pkgreconciler.Event {
	_, cancel := context.WithTimeout(ctx, pkgreconciler.DefaultTimeout)
	defer cancel()

	pa, err := c.podAutoscalerLister.PodAutoscalers(spa.Namespace).Get(spa.Name)
	if apierrs.IsNotFound(err) {
		message := fmt.Sprintf("The PodAutoscaler %v/%v was not found.", spa.Namespace, spa.Name)
		spa.Status.MarkPodAutoscalerStageNotReady(message)
		return nil
	} else if err != nil {
		spa.Status.MarkPodAutoscalerStageNotReady(err.Error())
		return err
	}

	// As long as the PodAutoscaler with the same name as StagePodAutoscaler exists, and both of the DesiredScale
	// the ActualScale are available, we propagate the values to StagePodAutoscaler.
	// When these values are set in StagePodAutoscaler, it will kick off the reconciliation loop of the
	// RolloutOrchestrator.
	//
	// The purpose of assigning these values to make sure the changes on PodAutoscaler can trigger the
	// reconciliation loop of the RolloutOrchestrator.
	if pa.Status.DesiredScale != nil && pa.Status.ActualScale != nil {
		spa.Status.ActualScale = pa.Status.ActualScale
		spa.Status.DesiredScale = pa.Status.DesiredScale
		spa.Status.MarkPodAutoscalerStageReady()
	} else {
		message := fmt.Sprintf("The ActualScale or DesiredScale for the PodAutoscaler %v/%v was not ready.",
			spa.Namespace, spa.Name)
		spa.Status.MarkPodAutoscalerStageNotReady(message)
	}
	return nil
}
