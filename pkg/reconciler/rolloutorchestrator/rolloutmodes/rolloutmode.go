/*
Copyright 2024 The Knative Authors

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

package rolloutmodes

import (
	"context"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	clientset "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
	"knative.dev/serving/pkg/apis/serving"
)

type Rollout struct {
	RolloutSteps []RolloutStep
}

func (r *Rollout) Reconcile(ctx context.Context, ro *v1.RolloutOrchestrator, revScalingUp, revScalingDown map[string]*v1.TargetRevision) (bool, error) {
	for index, step := range r.RolloutSteps {
		if ro.IsStageInProgress() || index == 0 {
			err := step.Execute(ctx, ro, revScalingUp, revScalingDown)
			if err != nil {
				return false, err
			}
		}
		// If spec.StageRevisionStatus is nil, check on if the number of replicas meets the conditions.
		if ro.IsStageInProgress() {
			ready, err := step.Verify(ctx, ro, revScalingUp, revScalingDown)
			if err != nil {
				return false, err
			}
			if ready {
				step.ModifyStatus(ro)
			} else {
				return false, nil
			}
		}
	}

	return true, nil
}

type ScaleUpStep struct {
	BaseScaleStep
}

func (s *ScaleUpStep) Execute(ctx context.Context, ro *v1.RolloutOrchestrator, revScalingUp, _ map[string]*v1.TargetRevision) error {
	// Create or update the StagePodAutoscaler for the revision scale up
	for _, revUp := range revScalingUp {
		if _, err := s.CreateOrUpdateSPARev(ctx, ro, revUp, true, UpdateSPAForRevUp); err != nil {
			return err
		}
	}
	return nil
}

func (s *ScaleUpStep) Verify(ctx context.Context, ro *v1.RolloutOrchestrator, revScalingUp, revScalingDown map[string]*v1.TargetRevision) (bool, error) {
	for _, revUp := range revScalingUp {
		spa, err := s.StagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).Get(revUp.RevisionName)
		if err != nil {
			return false, err
		}

		// spa.IsStageScaleInReady() returns true, as long as both DesireScale and ActualScale are available.
		if !spa.IsStageScaleInReady() || !IsStageScaleUpReady(spa, revUp) {
			// Create the stage pod autoscaler with the new maxScale set to
			// maxScale defined in the revision traffic, because scale up phase is not over, we cannot
			// scale down the old revision.
			// Create or update the stagePodAutoscaler for the revision to be scaled down, even if the scaling up
			// phase is not over.
			for _, valDown := range revScalingDown {
				if _, err = s.CreateOrUpdateSPARev(ctx, ro, valDown, false, UpdateSPAForRevDown); err != nil {
					return false, err
				}
			}
			return false, nil
		}
	}
	return true, nil
}

func (s *ScaleUpStep) ModifyStatus(ro *v1.RolloutOrchestrator) {
	ro.Status.MarkStageRevisionScaleUpReady()
}

type ScaleDownStep struct {
	BaseScaleStep
}

func (s *ScaleDownStep) Execute(ctx context.Context, ro *v1.RolloutOrchestrator, revScalingUp, revScalingDown map[string]*v1.TargetRevision) error {
	// In the maintenance mode, we start to scale down the old revisions, but we need to create the SPAs for the new revisions,
	// because Knative Serving launches the new revisions first. We need to leverage the SPAs to control the number of
	// the pods for the new revisions.
	// In the normal mode, we will always get the SPA for the new revision. The creation of the SPA will not be called.
	for _, revUp := range revScalingUp {
		_, err := s.StagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).Get(revUp.RevisionName)
		if apierrs.IsNotFound(err) {
			spa := &v1.StagePodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      revUp.RevisionName,
					Namespace: ro.Namespace,
					Labels: map[string]string{
						serving.RevisionLabelKey: revUp.RevisionName,
						serving.ServiceLabelKey:  ro.Name,
					},
					OwnerReferences: []metav1.OwnerReference{
						*kmeta.NewControllerRef(ro),
					},
				},
				Spec: v1.StagePodAutoscalerSpec{
					// If the SPA for the new revisions are created, start with 0 for the MinScale, and 1 for MaxScale.
					StageMinScale: ptr.Int32(0),
					StageMaxScale: ptr.Int32(0),
				},
			}
			_, err = s.Client.ServingV1().StagePodAutoscalers(ro.Namespace).Create(ctx, spa, metav1.CreateOptions{})
			if err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		// If there are existing SPAs for the new revisions, keep them the same as before.
	}

	if len(revScalingDown) != 0 {
		for _, valDown := range revScalingDown {
			_, err := s.CreateOrUpdateSPARev(ctx, ro, valDown, true, UpdateSPAForRevDown)
			if err != nil {
				return err
			}
		}
	} else if len(ro.Spec.InitialRevisions) != 0 {
		// Reset the stagePodAutoscaler for the initial target revision, since it has scaled down to 0, without
		// taking any traffic.
		// Min and Max scale in stagePodAutoscaler will be set to the same value as in the revision.
		for _, initRev := range ro.Spec.InitialRevisions {
			if _, found := revScalingUp[initRev.RevisionName]; !found {
				// If this revision is not found in the map for revisions scaling up, we need to scale down.
				revScaleDown := initRev.DeepCopy()
				revScaleDown.Percent = nil
				_, err := s.CreateOrUpdateSPARev(ctx, ro, revScaleDown, true, UpdateSPAForRevDown)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (s *ScaleDownStep) Verify(ctx context.Context, ro *v1.RolloutOrchestrator, _, revScalingDown map[string]*v1.TargetRevision) (bool, error) {
	if len(revScalingDown) != 0 {
		for _, valDown := range revScalingDown {
			_, err := s.CreateOrUpdateSPARev(ctx, ro, valDown, true, UpdateSPAForRevDown)
			if err != nil {
				return false, err
			}

			spa, err := s.StagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).Get(valDown.RevisionName)
			if err != nil {
				return false, err
			}

			if spa.Status.ReplicasTerminating == nil ||
				(spa.Status.ReplicasTerminating != nil && *spa.Status.ReplicasTerminating > 0) ||
				!IsStageScaleDownReady(spa, valDown) {
				return false, nil
			}
		}
	}

	return true, nil
}

func (s *ScaleDownStep) ModifyStatus(ro *v1.RolloutOrchestrator) {
	ro.Status.MarkStageRevisionScaleDownReady()
}

func NewRolloutModes(client clientset.Interface, stagePodAutoscalerLister listers.StagePodAutoscalerLister) map[string]*Rollout {
	rolloutMode := map[string]*Rollout{}
	baseScaleStep := BaseScaleStep{
		Client:                   client,
		StagePodAutoscalerLister: stagePodAutoscalerLister,
	}
	scaleUpStep := &ScaleUpStep{
		BaseScaleStep: baseScaleStep,
	}
	scaleDownStep := &ScaleDownStep{
		BaseScaleStep: baseScaleStep,
	}
	rolloutSteps := make([]RolloutStep, 0, 2)
	rolloutSteps = append(rolloutSteps, scaleUpStep)
	rolloutSteps = append(rolloutSteps, scaleDownStep)
	normalRollout := &Rollout{
		RolloutSteps: rolloutSteps,
	}
	rolloutMode[NormalMode] = normalRollout

	scaleUpMStep := &ScaleUpStep{
		BaseScaleStep: baseScaleStep,
	}
	scaleDownMStep := &ScaleDownStep{
		BaseScaleStep: baseScaleStep,
	}
	rolloutMSteps := make([]RolloutStep, 0, 2)
	rolloutMSteps = append(rolloutMSteps, scaleDownMStep)
	rolloutMSteps = append(rolloutMSteps, scaleUpMStep)
	maintenanceRollout := &Rollout{
		RolloutSteps: rolloutMSteps,
	}
	rolloutMode[MaintenanceMode] = maintenanceRollout
	return rolloutMode
}
