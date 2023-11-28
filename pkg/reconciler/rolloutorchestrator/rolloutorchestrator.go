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

// createOrUpdateEachSPAForRev create or update the StagePodAutoscaler, based on the specific (Stage)TargetRevision
// defined in the RolloutOrchestrator.
func (c *Reconciler) createOrUpdateEachSPAForRev(ctx context.Context, ro *v1.RolloutOrchestrator,
	targetRev *v1.TargetRevision, scaleUpReady bool) (*v1.StagePodAutoscaler, error) {
	spa, err := c.stagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).Get(targetRev.RevisionName)
	if apierrs.IsNotFound(err) {
		return c.createStagePA(ctx, ro, targetRev, scaleUpReady)
	}
	if err != nil {
		return spa, err
	}
	return c.client.ServingV1().StagePodAutoscalers(ro.Namespace).Update(ctx,
		updateWithTargetReplicas(spa, targetRev, scaleUpReady), metav1.UpdateOptions{})
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, ro *v1.RolloutOrchestrator) pkgreconciler.Event {
	ctx, cancel := context.WithTimeout(ctx, pkgreconciler.DefaultTimeout)
	defer cancel()

	// If spec.StageRevisionStatus is nil, do nothing.
	if len(ro.Spec.StageTargetRevisions) == 0 {
		return nil
	}

	stageTargetRevisions := ro.Spec.StageTargetRevisions
	revScalingUp, revScalingDown, err := retrieveRevsUpDown(stageTargetRevisions)
	if err != nil {
		return err
	}
	// Create the stagePodAutoscaler for the revision to be scaled up
	_, err = c.createOrUpdateEachSPAForRev(ctx, ro, revScalingUp, false)
	if err != nil {
		return err
	}

	// If spec.StageRevisionStatus is nil, check on if the number of replicas meets the conditions.
	if ro.IsStageInProgress() {
		spa, err := c.stagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).Get(revScalingUp.RevisionName)
		if err != nil {
			return err
		}
		if !spa.IsStageScaleInReady() || !isStageScaleUpReady(spa, revScalingUp) {
			// Create the stage pod autoscaler with the new maxScale set to
			// maxScale defined in the revision traffic, because scale up phase is not over, we cannot
			// scale down the old revision.
			_, err = c.createOrUpdateEachSPAForRev(ctx, ro, revScalingDown, false)
			if err != nil {
				return err
			}
			return nil
		}

		ro.Status.MarkStageRevisionScaleUpReady()

		// Create the stage pod autoscaler with the new maxScale set to targetScale defined
		// in the revision traffic. Scaling up phase is over, we are able to scale down.
		// Create the stagePodAutoscaler for the revision to be scaled up
		_, err = c.createOrUpdateEachSPAForRev(ctx, ro, revScalingDown, true)
		if err != nil {
			return err
		}

		spa, err = c.stagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).Get(revScalingDown.RevisionName)
		if err != nil {
			return err
		}
		if !isStageScaleDownReady(spa, revScalingDown) {
			return nil
		}

		ro.Status.MarkStageRevisionScaleDownReady()

		// When the number of replicas of the new and old revision meets the conditions, set the status to stage ready.
		stageCleaned := removeNonTrafficRev(ro.Spec.StageTargetRevisions)
		ro.Status.SetStageRevisionStatus(stageCleaned)
		ro.Status.MarkStageRevisionReady()

		if lastStageComplete(ro.Status.StageRevisionStatus, ro.Spec.TargetRevisions) {
			ro.Status.MarkLastStageRevisionComplete()
			return nil
		}
		ro.Status.MarkLastStageRevisionInComplete()
		return nil
	}

	if ro.IsStageReady() && ro.IsInProgress() && !lastStageComplete(ro.Status.StageRevisionStatus,
		ro.Spec.StageTargetRevisions) {
		// Start to move to a new stage.
		ro.Status.LaunchNewStage()
	}

	return nil
}

func (c *Reconciler) createStagePA(ctx context.Context, so *v1.RolloutOrchestrator, revision *v1.TargetRevision,
	scaleUpReady bool) (*v1.StagePodAutoscaler, error) {
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
	spa = updateWithTargetReplicas(spa, revision, scaleUpReady)
	return c.client.ServingV1().StagePodAutoscalers(so.Namespace).Create(ctx, spa, metav1.CreateOptions{})
}

func removeNonTrafficRev(ts []v1.TargetRevision) []v1.TargetRevision {
	result := make([]v1.TargetRevision, 0)
	for _, r := range ts {
		if r.Percent != nil && *r.Percent != 0 {
			result = append(result, r)
		}
	}
	return result
}

func targetRevisionEqual(currentStatusRevisions, finalTargetRevisions []v1.TargetRevision) bool {
	if *finalTargetRevisions[0].Percent != 100 {
		return false
	}
	for _, r := range currentStatusRevisions {
		if *r.Percent == 100 && r.RevisionName == finalTargetRevisions[0].RevisionName {
			return true
		}
	}
	return false
}

func updateWithTargetReplicas(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision,
	scaleUpReady bool) *v1.StagePodAutoscaler {
	min := int32(0)
	max := int32(math.MaxInt32)
	if revision.MinScale != nil {
		min = *revision.MinScale
	}

	if revision.MaxScale != nil {
		max = *revision.MaxScale
	}

	if revision.IsRevScalingUp() {
		if revision.TargetReplicas == nil {
			spa.Spec.StageMinScale = revision.MinScale
			spa.Spec.StageMaxScale = revision.MaxScale
			return spa
		}
		targetReplicas := *revision.TargetReplicas
		spa.Spec.StageMaxScale = revision.MaxScale
		if targetReplicas < min && *revision.Percent < int64(100) {
			spa.Spec.StageMinScale = ptr.Int32(targetReplicas)
		} else {
			spa.Spec.StageMinScale = revision.MinScale
		}
		return spa
	}

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

	return spa
}

func retrieveRevsUpDown(targetRevs []v1.TargetRevision) (*v1.TargetRevision, *v1.TargetRevision, error) {
	upIndex, downIndex := -1, -1
	for i, rev := range targetRevs {
		if rev.IsRevScalingUp() {
			upIndex = i
		} else if rev.IsRevScalingDown() {
			downIndex = i
		}
	}
	if upIndex == -1 || downIndex == -1 {
		return nil, nil, fmt.Errorf("unable to find the revision to scale up or down in the target revisions")
	}
	return &targetRevs[upIndex], &targetRevs[downIndex], nil
}

func lastStageComplete(stageRevisionStatus, finalTargetRevs []v1.TargetRevision) bool {
	return equality.Semantic.DeepEqual(stageRevisionStatus, finalTargetRevs) ||
		targetRevisionEqual(stageRevisionStatus, finalTargetRevs)
}

func isStageScaleUpReady(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision) bool {
	if spa.Status.DesiredScale == nil || spa.Status.ActualScale == nil {
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
		if *spa.Status.DesiredScale == *spa.Status.ActualScale && *spa.Status.ActualScale >= min && *spa.Status.ActualScale <= max {
			return true
		}
		return false
	}
	if *spa.Status.DesiredScale >= *revision.TargetReplicas && *spa.Status.ActualScale >= *revision.TargetReplicas {
		return true
	} else if *spa.Status.DesiredScale == *revision.TargetReplicas && *spa.Status.DesiredScale == *spa.Status.ActualScale {
		return true
	} else if *spa.Status.DesiredScale >= min && *spa.Status.DesiredScale == *spa.Status.ActualScale {
		return true
	}

	return false
}

func isStageScaleDownReady(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision) bool {
	if spa.Status.DesiredScale == nil || spa.Status.ActualScale == nil {
		return false
	}
	if revision.TargetReplicas == nil {
		max := int32(math.MaxInt32)
		if revision.MaxScale != nil {
			max = *revision.MaxScale
		}
		return bothValuesUnderTargetValue(*spa.Status.DesiredScale, *spa.Status.ActualScale, max)
	}

	return bothValuesUnderTargetValue(*spa.Status.DesiredScale, *spa.Status.ActualScale, *revision.TargetReplicas)
}

func bothValuesUnderTargetValue(desire, actual, target int32) bool {
	return desire <= target && actual <= target
}
