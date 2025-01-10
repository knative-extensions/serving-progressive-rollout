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

package strategies

import (
	"context"
	"math"
	"strings"
	"time"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	clientset "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
	"knative.dev/serving/pkg/apis/serving"
)

var (
	// AvailabilityStrategy one strategy to roll out the new revision of the knative service, making sure the service is
	// always available.
	AvailabilityStrategy = "availability"
	// ResourceUtilStrategy is one strategy to roll out the new revision of the knative service, making sure the resource
	// is used optimized. It is possible that the service can have the downtime.
	ResourceUtilStrategy = "resourceutil"
)

// The RolloutStep interface defines all the functions, that are necessary to call to accomplish the rollout step.
// Currently, there are two type of steps, it could be either scaling up or scaling down.
type RolloutStep interface {
	// Execute function create or update the SPAs for the revisions to either scale up or down.
	Execute(ctx context.Context, ro *v1.RolloutOrchestrator, revScalingUp, revScalingDown map[string]*v1.TargetRevision) error
	// Verify function checks whether it reaches the completion of the scaling up or down for the revisions.
	Verify(ctx context.Context, ro *v1.RolloutOrchestrator, revScalingUp, revScalingDown map[string]*v1.TargetRevision,
		enqueueAfter func(interface{}, time.Duration)) (bool, error)
	// ModifyStatus function changes the status of the ro accordingly after the completion of the scaling up or down for the
	// revisions.
	ModifyStatus(ro *v1.RolloutOrchestrator, ready bool)
}

// The BaseScaleStep struct defines golang clients, that are necessary to access the kubernetes resources.
// It also consists of the functions to create or update the SPAs for the revisions.
type BaseScaleStep struct {
	Client                   clientset.Interface
	Kubeclient               kubernetes.Interface
	StagePodAutoscalerLister listers.StagePodAutoscalerLister
}

type updateSPAForRev func(*v1.StagePodAutoscaler, *v1.TargetRevision, bool) *v1.StagePodAutoscaler

func (r *BaseScaleStep) CreateOrUpdateSPARev(ctx context.Context, ro *v1.RolloutOrchestrator,
	targetRev *v1.TargetRevision, scaleUpReady bool, fn updateSPAForRev) (*v1.StagePodAutoscaler, error) {
	spa, err := r.StagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).Get(targetRev.RevisionName)
	if apierrs.IsNotFound(err) {
		return r.createStagePA(ctx, ro, targetRev, scaleUpReady, fn)
	}
	if err != nil {
		return spa, err
	}
	return r.Client.ServingV1().StagePodAutoscalers(ro.Namespace).Update(ctx,
		fn(spa, targetRev, scaleUpReady), metav1.UpdateOptions{})
}

func (r *BaseScaleStep) createStagePA(ctx context.Context, ro *v1.RolloutOrchestrator, revision *v1.TargetRevision,
	scaleUpReady bool, fn updateSPAForRev) (*v1.StagePodAutoscaler, error) {
	spa := CreateBaseStagePodAutoscaler(ro, revision)
	spa = fn(spa, revision, scaleUpReady)
	return r.Client.ServingV1().StagePodAutoscalers(ro.Namespace).Create(ctx, spa, metav1.CreateOptions{})
}

// CreateBaseStagePodAutoscaler returns the basic spa(StagePodAutoscaler), base
// on the RolloutOrchestrator and the revision.
func CreateBaseStagePodAutoscaler(ro *v1.RolloutOrchestrator, revision *v1.TargetRevision) (spa *v1.StagePodAutoscaler) {
	spa = &v1.StagePodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revision.RevisionName,
			Namespace: ro.Namespace,
			Labels: map[string]string{
				serving.RevisionLabelKey: revision.RevisionName,
				serving.ServiceLabelKey:  ro.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(ro),
			},
		},
		Spec: v1.StagePodAutoscalerSpec{
			StageMinScale: revision.MinScale,
			StageMaxScale: revision.MaxScale,
		},
	}
	return
}

// UpdateSPAForRevUp update the SPA(StagePodAutoscaler) for the revision scaling up, based on the TargetReplicas
// min & max scales defined in the Knative Service.
func UpdateSPAForRevUp(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision, _ bool) *v1.StagePodAutoscaler {
	// For revisions scaling up, the StageMaxScale is always set to the final MaxScale.
	spa.Spec.StageMaxScale = revision.MaxScale
	minR := getMinScale(revision)

	// TargetReplicas is nil, when either the revision has 0% traffic assigned or 100% traffic assigned.
	// It will be either the old revision scales down to 0 or the new revision scale up to replace the old revision.
	// In either case, the spa pick up the StageMinScale and StageMaxScale scales from the knative service.
	if revision.TargetReplicas == nil {
		spa.Spec.StageMinScale = revision.MinScale
		return spa
	}
	targetReplicas := *revision.TargetReplicas
	if targetReplicas < minR && *revision.Percent < int64(100) {
		// If the less than of 100% traffic is assigned to this revision or targetReplicas is less than minscale,
		// set StageMinScale directly to targetReplicas.
		spa.Spec.StageMinScale = ptr.Int32(targetReplicas)
	} else {
		// If the 100% traffic is assigned to this revision or targetReplicas is equal to or greater than min,
		// set StageMinScale directly to the final MinScale from the knative service.
		spa.Spec.StageMinScale = revision.MinScale
	}
	return spa
}

// UpdateSPAForRevDown update the SPA(StagePodAutoscaler) for the revision scaling down, based on the TargetReplicas
// min & max scales defined in the Knative Service, if the scaleUpReady is true.
//
// If the scaleUpReady is false, no change to the SPA(StagePodAutoscaler).
func UpdateSPAForRevDown(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision,
	scaleUpReady bool) *v1.StagePodAutoscaler {
	if !scaleUpReady {
		return spa
	}

	if len(strings.TrimSpace(revision.RevisionName)) == 0 {
		return spa
	}

	minR := getMinScale(revision)
	maxR := getMaxScale(revision)

	// If Percent is empty, it means the old revision has reduced the traffic down to 0%.
	if revision.Percent == nil {
		// We need to set a limit for the max scale, because 0 mean no limit.
		spa.Spec.StageMaxScale = ptr.Int32(1)
		spa.Spec.StageMinScale = ptr.Int32(0)
		return spa
	}

	// If targetReplicas is equal to or greater than maxScale, StageMinScale and StageMaxScale are set to the final
	// MinScale and MaxScale.
	if revision.TargetReplicas == nil || *revision.TargetReplicas >= maxR {
		spa.Spec.StageMinScale = revision.MinScale
		spa.Spec.StageMaxScale = revision.MaxScale
		return spa
	}

	// We need to set a limit for the max scale, because 0 mean no limit.
	if *revision.TargetReplicas == 0 {
		spa.Spec.StageMaxScale = ptr.Int32(1)
		spa.Spec.StageMinScale = ptr.Int32(0)
		return spa
	}

	targetReplicas := *revision.TargetReplicas
	// If targetReplicas is less than maxScale, StageMaxScale is set to targetReplicas.
	spa.Spec.StageMaxScale = ptr.Int32(targetReplicas)

	// If targetReplicas is less than minScale, StageMinScale is set to targetReplicas.
	if targetReplicas < minR {
		spa.Spec.StageMinScale = ptr.Int32(targetReplicas)
		return spa
	}

	// If targetReplicas is larger than or equal to minScale, StageMinScale is set to final MinScale.
	spa.Spec.StageMinScale = revision.MinScale
	return spa
}

func getMinScale(revision *v1.TargetRevision) (min int32) {
	if revision.MinScale != nil {
		min = *revision.MinScale
	}
	return
}

func getMaxScale(revision *v1.TargetRevision) (maxR int32) {
	maxR = int32(math.MaxInt32)
	if revision.MaxScale != nil {
		maxR = *revision.MaxScale
	}
	return
}

// IsStageScaleUpReady decides whether the scaling up has completed or on the way for the current stage, based
// on the revision and the spa(StagePodAutoscaler).
func IsStageScaleUpReady(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision) bool {
	if spa.Status.DesiredScale == nil || spa.Status.ActualScale == nil {
		return false
	}
	minR := getMinScale(revision)
	maxR := getMaxScale(revision)
	if revision.TargetReplicas == nil {
		// For revision scaling up without TargetReplicas, it means this revision will be assigned 100% of the traffic.
		return actualScaleBetweenMinMax(spa, minR, maxR)
	}

	// There are two modes to scale up and down the replicas of the revisions:
	// 1. No traffic. Knative Service specifies both min and max scales for the revision. We need to control the
	// StageMinScale and StageMaxScale to make sure the replicas increase or decrease. In this case, we need to
	// precisely make sure both DesiredScale and ActualScale are equal to or greater than TargetReplicas to determine
	// scaling up phase is over. TargetReplicas is no larger than minScale, because revision runs at the number of
	// minScale. We need to first scale up the new revision, make sure it run at the correct number, and scale down the
	// old revision. Shifting traffic does not change anything in terms of the number of replicas.
	// 2. Traffic driven. In this case, TargetReplicas is larger than minScale and lower than or equal to maxScale
	// for the revision. We need to change the number of the replicas by shifting the traffic. As long as we know the
	// new revision is on the way of scaling up, we are able to start the scaling down phase as well.
	if minR >= *revision.TargetReplicas {
		// This is for the first mode.
		return *spa.Status.DesiredScale >= *revision.TargetReplicas && *spa.Status.ActualScale >= *revision.TargetReplicas
	}

	// This is for the second mode.
	return *spa.Status.DesiredScale >= minR && *spa.Status.ActualScale >= minR
}

// IsStageScaleDownReady decides whether the scaling down has completed for the current stage, based
// on the revision and the spa(StagePodAutoscaler).
func IsStageScaleDownReady(spa *v1.StagePodAutoscaler, revision *v1.TargetRevision) bool {
	if spa.Status.DesiredScale == nil || spa.Status.ActualScale == nil {
		return false
	}
	if revision.TargetReplicas == nil {
		// For revision scaling down without TargetReplicas, it means this revision will be assigned the same
		// traffic percentage as the previous stage.
		max := getMaxScale(revision)
		return bothValuesUnderTargetValue(*spa.Status.DesiredScale, *spa.Status.ActualScale, max)
	}
	return bothValuesUnderTargetValue(*spa.Status.DesiredScale, *spa.Status.ActualScale, *revision.TargetReplicas)
}

func bothValuesUnderTargetValue(desire, actual, target int32) bool {
	return desire <= target && actual <= target
}

func actualScaleBetweenMinMax(spa *v1.StagePodAutoscaler, min, max int32) bool {
	return *spa.Status.ActualScale >= min && *spa.Status.ActualScale <= max
}
