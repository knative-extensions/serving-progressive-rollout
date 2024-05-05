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
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	pkgreconciler "knative.dev/pkg/reconciler"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	clientset "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	roreconciler "knative.dev/serving-progressive-rollout/pkg/client/injection/reconciler/serving/v1/rolloutorchestrator"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/rolloutorchestrator/rolloutmodes"
	"knative.dev/serving/pkg/apis/serving"
	servinglisters "knative.dev/serving/pkg/client/listers/serving/v1"
)

// Reconciler implements controller.Reconciler for RolloutOrchestrator resources.
type Reconciler struct {
	client clientset.Interface

	// lister indexes properties about StagePodAutoscaler
	stagePodAutoscalerLister listers.StagePodAutoscalerLister
	deploymentLister         appsv1listers.DeploymentLister
	revisionLister           servinglisters.RevisionLister
	rolloutMode              map[string]*rolloutmodes.Rollout
	enqueueAfter             func(interface{}, time.Duration)
}

// Check that our Reconciler implements roreconciler.Interface
var _ roreconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, ro *v1.RolloutOrchestrator) pkgreconciler.Event {
	ctx, cancel := context.WithTimeout(ctx, pkgreconciler.DefaultTimeout)
	defer cancel()
	defer r.cleanUpSPAs(ctx, ro)

	// If spec.StageRevisionStatus is nil, do nothing.
	if len(ro.Spec.StageTargetRevisions) == 0 {
		return nil
	}

	// Spec.StageTargetRevisions in the RolloutOrchestrator defines what the current stage looks like, in terms
	// of the available revisions, and their name, traffic percentage, target number of replicas, whether it
	// scales up or down, min and max scales defined by the Knative Service.
	stageTargetRevisions := ro.Spec.StageTargetRevisions
	revScalingUp, revScalingDown, err := RetrieveRevsUpDown(stageTargetRevisions)

	if err != nil {
		return err
	}

	rollout := r.rolloutMode[ro.Spec.RolloutMode]
	ready, err := rollout.Reconcile(ctx, ro, revScalingUp, revScalingDown, r.enqueueAfter)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}

	if ro.IsStageInProgress() {
		// Clean up and set the status of the StageRevision. It means the orchestrator has accomplished this stage.
		if len(ro.Spec.TargetRevisions) < len(stageTargetRevisions) {
			stageCleaned := RemoveNonTrafficRev(stageTargetRevisions)
			ro.Status.SetStageRevisionStatus(stageCleaned)
		} else {
			ro.Status.SetStageRevisionStatus(stageTargetRevisions)
		}

		ro.Status.MarkStageRevisionReady()
		if LastStageComplete(ro.Status.StageRevisionStatus, ro.Spec.TargetRevisions) {
			ro.Status.MarkLastStageRevisionComplete()
			return nil
		}
		ro.Status.MarkLastStageRevisionInComplete()
		return nil
	}

	if ro.IsStageReady() && ro.IsInProgress() && !LastStageComplete(ro.Status.StageRevisionStatus,
		ro.Spec.TargetRevisions) {
		// Start to move to the next stage.
		ro.Status.LaunchNewStage()
	}

	return nil
}

func (r *Reconciler) cleanUpSPAs(ctx context.Context, ro *v1.RolloutOrchestrator) {
	records := map[string]bool{}
	// Get the list of all the revisions for the knative service.
	revList, err := r.revisionLister.Revisions(ro.Namespace).List(labels.SelectorFromSet(labels.Set{
		serving.ConfigurationLabelKey: ro.Name,
		serving.ServiceLabelKey:       ro.Name,
	}))

	if err == nil && len(revList) > 0 {
		for _, rev := range revList {
			records[rev.Name] = true
		}
	}

	// Get the list of all the SPAs for the knative service.
	spaList, err := r.stagePodAutoscalerLister.StagePodAutoscalers(ro.Namespace).List(labels.SelectorFromSet(labels.Set{
		serving.ServiceLabelKey: ro.Name,
	}))
	if err == nil && len(spaList) > 0 {
		for _, spa := range spaList {
			// The SPA and the revision share the same name. If the revision is gone, delete the SPA.
			if !records[spa.Name] {
				r.client.ServingV1().StagePodAutoscalers(ro.Namespace).Delete(ctx, spa.Name, metav1.DeleteOptions{})
			}
		}
	}
}

// RemoveNonTrafficRev removes the redundant TargetRevision from the list of TargetRevisions.
func RemoveNonTrafficRev(ts []v1.TargetRevision) []v1.TargetRevision {
	result := make([]v1.TargetRevision, 0, len(ts))
	for _, r := range ts {
		if r.Percent != nil && (*r.Percent != 0 || (*r.Percent == 0 && r.LatestRevision != nil && *r.LatestRevision)) {
			result = append(result, r)
		}
	}
	if len(result) < len(ts) {
		// We need to find out the revision to scale down, since we remove the revision previously scaling down.
		for i := len(result) - 1; i >= 0; i-- {
			if result[i].Direction == "stay" {
				result[i].Direction = v1.DirectionDown
				break
			}
		}
	}
	return result
}

func targetRevisionEqual(currentStatusRevisions, finalTargetRevisions []v1.TargetRevision) bool {
	if *finalTargetRevisions[0].Percent != 100 {
		return false
	}
	for _, r := range currentStatusRevisions {
		if r.Percent != nil && *r.Percent == 100 && r.RevisionName == finalTargetRevisions[0].RevisionName {
			return true
		}
	}
	return false
}

// RetrieveRevsUpDown returns two list of revisions scaling up and down based on the input TargetRevisions.
func RetrieveRevsUpDown(targetRevs []v1.TargetRevision) (map[string]*v1.TargetRevision, map[string]*v1.TargetRevision, error) {
	targetRevsUp, targetRevsDown := make(map[string]*v1.TargetRevision), make(map[string]*v1.TargetRevision)
	for _, rev := range targetRevs {
		if rev.IsRevScalingUp() {
			targetRevsUp[rev.RevisionName] = rev.DeepCopy()
		} else if rev.IsRevScalingDown() {
			targetRevsDown[rev.RevisionName] = rev.DeepCopy()
		}
	}
	if len(targetRevsUp) == 0 {
		return targetRevsUp, targetRevsDown, fmt.Errorf("unable to find the revision to scale up in the target revisions %v", targetRevs)
	}

	return targetRevsUp, targetRevsDown, nil
}

// LastStageComplete decides whether the last stage of the progressive upgrade is complete or not.
func LastStageComplete(stageRevisionStatus, finalTargetRevs []v1.TargetRevision) bool {
	return equality.Semantic.DeepEqual(stageRevisionStatus, finalTargetRevs) ||
		targetRevisionEqual(stageRevisionStatus, finalTargetRevs)
}
