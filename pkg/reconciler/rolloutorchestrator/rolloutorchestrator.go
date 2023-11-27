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
	"fmt"
	"math"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
	pkgreconciler "knative.dev/pkg/reconciler"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	clientset "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	roreconciler "knative.dev/serving-progressive-rollout/pkg/client/injection/reconciler/serving/v1/rolloutorchestrator"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
	"knative.dev/serving/pkg/apis/serving"
)

// Reconciler implements controller.Reconciler for Configuration resources.
type Reconciler struct {
	client clientset.Interface

	// listers index properties about resources
	stagePodAutoscalerLister listers.StagePodAutoscalerLister
}

// Check that our Reconciler implements soreconciler.Interface
var _ roreconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, so *v1.RolloutOrchestrator) pkgreconciler.Event {
	ctx, cancel := context.WithTimeout(ctx, pkgreconciler.DefaultTimeout)
	defer cancel()

	// If spec.StageRevisionStatus is nil, do nothing.
	if len(so.Spec.StageTargetRevisions) == 0 {
		return nil
	}

	// Create the stagePodAutoscaler for the revision to be scaled up
	for _, revision := range so.Spec.StageTargetRevisions {
		if revision.Direction == "" || revision.Direction == "up" {
			spa, err := c.stagePodAutoscalerLister.StagePodAutoscalers(so.Namespace).Get(revision.RevisionName)
			if apierrs.IsNotFound(err) {
				c.createStagePA(ctx, so, &revision, false)
				return nil
			} else if err != nil {
				return nil
			} else {
				spa = updateWithTargetReplicas(spa, &revision, false)
				c.client.ServingV1().StagePodAutoscalers(so.Namespace).Update(ctx, spa, metav1.UpdateOptions{})
			}
		}
	}

	// If spec.StageRevisionStatus is nil, check on if the number of replicas meets the conditions.
	if so.IsStageInProgress() {
		if !c.checkStageScaleUpReady(ctx, so) {
			// Create the stage pod autoscaler with the new maxScale set to
			// maxScale defined in the revision traffic, because scale up phase is not over, we cannot
			// scale down the old revision.
			for _, revision := range so.Spec.StageTargetRevisions {
				if revision.Direction == "down" {
					spa, err := c.stagePodAutoscalerLister.StagePodAutoscalers(so.Namespace).Get(revision.RevisionName)
					if apierrs.IsNotFound(err) {
						c.createStagePA(ctx, so, &revision, false)
						return nil
					} else if err != nil {
						return nil
					} else {
						spa = updateWithTargetReplicas(spa, &revision, false)
						c.client.ServingV1().StagePodAutoscalers(so.Namespace).Update(ctx, spa, metav1.UpdateOptions{})
					}
				}
			}
			return nil
		}

		so.Status.MarkStageRevisionScaleUpReady()
		// Create the stage pod autoscaler with the new maxScale set to targetScale defined
		// in the revision traffic. Scaling up phase is over, we are able to scale down.
		// Create the stagePodAutoscaler for the revision to be scaled up
		for _, revision := range so.Spec.StageTargetRevisions {
			if revision.Direction == "down" {
				spa, err := c.stagePodAutoscalerLister.StagePodAutoscalers(so.Namespace).Get(revision.RevisionName)
				if apierrs.IsNotFound(err) {
					c.createStagePA(ctx, so, &revision, true)
					return nil
				} else if err != nil {
					return nil
				} else {
					spa = updateWithTargetReplicas(spa, &revision, true)
					c.client.ServingV1().StagePodAutoscalers(so.Namespace).Update(ctx, spa, metav1.UpdateOptions{})
				}
			}
		}

		if !c.checkStageScaleDownReady(ctx, so) {
			return nil
		}

		so.Status.MarkStageRevisionScaleDownReady()

		// When the number of replicas of the new and old revision meets the conditions, set the status to stage ready.
		stageCleaned := cleanRedundency(so.Spec.StageTargetRevisions)
		so.Status.SetStageRevisionStatus(stageCleaned)
		so.Status.MarkStageRevisionReady()
		if equality.Semantic.DeepEqual(so.Status.StageRevisionStatus, so.Spec.TargetRevisions) || LatestEqual(so.Status.StageRevisionStatus, so.Spec.TargetRevisions) {
			so.Status.MarkLastStageRevisionComplete()

		} else {
			so.Status.MarkLastStageRevisionInComplete("RevisionRollingInProgress", "Still rolling the new revision.")
		}
		return nil
	} else if so.IsStageReady() {
		if so.IsInProgress() {
			if !equality.Semantic.DeepEqual(so.Status.StageRevisionStatus, so.Spec.StageTargetRevisions) && !LatestEqual(so.Status.StageRevisionStatus, so.Spec.TargetRevisions) {
				// Start to move to a new stage.
				so.Status.MarkStageRevisionScaleUpInProgress("StageRevisionStart", "Start to roll out a new stage.")
				so.Status.MarkStageRevisionScaleDownInProgress("StageRevisionStart", "Start to roll out a new stage.")
				so.Status.MarkStageRevisionInProgress("StageRevisionStart", "Start to roll out a new stage.")
				so.Status.MarkLastStageRevisionInComplete("RevisionRollingInProgress", "Still rolling the new revision.")
			}
		}
	}

	return nil
}

func cleanRedundency(ts []v1.TargetRevision) []v1.TargetRevision {
	result := []v1.TargetRevision{}
	for _, r := range ts {
		if *r.Percent != 0 {
			result = append(result, r)
		}
	}
	return result
}

func LatestEqual(t1, t2 []v1.TargetRevision) bool {
	if *t2[0].Percent != 100 {
		return false
	}
	for _, r := range t1 {
		if *r.Percent == 100 && r.RevisionName == t2[0].RevisionName {
			return true
		}
	}
	return false
}

func (c *Reconciler) createStagePA(ctx context.Context, so *v1.RolloutOrchestrator, revision *v1.TargetRevision, scaleUpReady bool) error {
	spa := &v1.StagePodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revision.RevisionName,
			Namespace: so.Namespace,
			Labels:    map[string]string{serving.RevisionLabelKey: revision.RevisionName},
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(so),
			},
		},
		Spec: v1.StagePodAutoscalerSpec{
			StageMinScale: revision.MinScale,
			StageMaxScale: revision.MaxScale,
		},
	}
	logger := logging.FromContext(ctx)
	logger.Info("go to create ")
	spa = updateWithTargetReplicas(spa, revision, scaleUpReady)

	logger.Info("display the spec of logger")
	if spa.Spec.StageMinScale != nil {
		logger.Info(*spa.Spec.StageMinScale)
	}
	if spa.Spec.StageMaxScale != nil {
		logger.Info(*spa.Spec.StageMaxScale)
	}

	c.client.ServingV1().StagePodAutoscalers(so.Namespace).Create(ctx, spa, metav1.CreateOptions{})
	return nil
}

func updateWithTargetReplicas(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision,
	scaleUpReady bool) *v1.StagePodAutoscaler {
	fmt.Println("run the updateWithTargetReplicas")

	min := int32(0)
	max := int32(math.MaxInt32)
	if revision.MinScale != nil {
		min = *revision.MinScale
	}

	if revision.MaxScale != nil {
		max = *revision.MaxScale
	}

	if revision.Direction == "" || revision.Direction == "up" {
		if revision.TargetReplicas == nil {
			spa.Spec.StageMinScale = revision.MinScale
			spa.Spec.StageMaxScale = revision.MaxScale
			return spa
		}
		targetReplicas := *revision.TargetReplicas
		spa.Spec.StageMaxScale = revision.MaxScale
		fmt.Println("check the up min scale")
		if targetReplicas < min && *revision.Percent < int64(100) {
			fmt.Println("set to target scale")
			fmt.Println("target")
			fmt.Println(targetReplicas)
			fmt.Println("min")
			fmt.Println(min)
			fmt.Println("percent")
			fmt.Println(*revision.Percent)
			spa.Spec.StageMinScale = ptr.Int32(targetReplicas)
		} else {
			fmt.Println("set to min scale")
			spa.Spec.StageMinScale = revision.MinScale
		}

	} else if revision.Direction == "down" {
		if scaleUpReady {
			if revision.TargetReplicas == nil {
				spa.Spec.StageMinScale = revision.MinScale
				spa.Spec.StageMaxScale = revision.MaxScale
				return spa
			}
			targetReplicas := *revision.TargetReplicas
			if targetReplicas < max {
				spa.Spec.StageMaxScale = ptr.Int32(targetReplicas)
			} else {
				spa.Spec.StageMaxScale = revision.MaxScale
			}

			if targetReplicas < min {
				spa.Spec.StageMinScale = ptr.Int32(targetReplicas)
			} else {
				spa.Spec.StageMinScale = revision.MinScale
			}
		}
		//} else {
		//	spa.Spec.MaxScale = revision.MaxScale
		//	spa.Spec.MinScale = revision.MinScale
		//}
	}
	return spa
}

func (c *Reconciler) checkStageScaleUpReady(ctx context.Context, so *v1.RolloutOrchestrator) bool {
	logger := logging.FromContext(ctx)
	logger.Info("checkStageScaleUpReady checkStageScaleUpReady checkStageScaleUpReady checkStageScaleUpReady")
	for _, revision := range so.Spec.StageTargetRevisions {
		pa, err := c.stagePodAutoscalerLister.StagePodAutoscalers(so.Namespace).Get(revision.RevisionName)
		if err != nil {
			logger.Info("checkStageScaleUpReady there is error checkStageScaleUpReady checkStage")
			logger.Info(err)
			return false
		}
		if revision.Direction == "" || revision.Direction == "up" {
			logger.Info("checkStageScaleUpReady check the revision")
			logger.Info(pa.IsStageScaleInReady())

			if !pa.IsStageScaleInReady() {
				return false
			}
			min := int32(0)
			max := int32(math.MaxInt32)
			if revision.MinScale != nil {
				min = *revision.MinScale
			}

			if revision.MaxScale != nil {
				max = *revision.MaxScale
			}
			if revision.TargetReplicas == nil {

				if *pa.Status.DesiredScale == *pa.Status.ActualScale && *pa.Status.ActualScale >= min && *pa.Status.ActualScale <= max {
					return true
				}
				return false
			}
			if *pa.Status.DesiredScale >= *revision.TargetReplicas && *pa.Status.ActualScale >= *revision.TargetReplicas {
				return true
			} else if *pa.Status.DesiredScale == *revision.TargetReplicas && *pa.Status.DesiredScale == *pa.Status.ActualScale {
				return true
			} else if *pa.Status.DesiredScale >= min && *pa.Status.DesiredScale == *pa.Status.ActualScale {
				return true
			}

			//else if *pa.Status.DesiredScale < *revision.TargetReplicas && *pa.Status.ActualScale < *revision.TargetReplicas {
			//	if *pa.Status.DesiredScale == *pa.Status.ActualScale {
			//		return true
			//	}
			//}
		}

	}
	return false
}

func (c *Reconciler) checkStageScaleDownReady(ctx context.Context, so *v1.RolloutOrchestrator) bool {
	logger := logging.FromContext(ctx)
	logger.Info("mark checkStageScaleDownReady ready")
	for _, revision := range so.Spec.StageTargetRevisions {
		pa, err := c.stagePodAutoscalerLister.StagePodAutoscalers(so.Namespace).Get(revision.RevisionName)
		if err != nil {
			return false
		}
		if revision.Direction == "down" {
			if revision.TargetReplicas == nil {
				max := int32(math.MaxInt32)

				if revision.MaxScale != nil {
					max = *revision.MaxScale
				}
				if *pa.Status.DesiredScale > max || *pa.Status.ActualScale > max {
					return false
				}

				return false
			}
			//if !pa.IsStageScaleInReady() {
			//	return false
			//}
			if *pa.Status.DesiredScale > *revision.TargetReplicas || *pa.Status.ActualScale > *revision.TargetReplicas {
				return false
			}
		}
	}
	return true
}
