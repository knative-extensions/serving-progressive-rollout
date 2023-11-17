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
	"fmt"
	"math"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"knative.dev/pkg/logging"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/service/resources"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"

	"knative.dev/pkg/controller"
	"knative.dev/pkg/ptr"
	pkgreconciler "knative.dev/pkg/reconciler"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	clientset "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingclientset "knative.dev/serving/pkg/client/clientset/versioned"
	ksvcreconciler "knative.dev/serving/pkg/client/injection/reconciler/serving/v1/service"
	palisters "knative.dev/serving/pkg/client/listers/autoscaling/v1alpha1"
	servinglisters "knative.dev/serving/pkg/client/listers/serving/v1"
	servingService "knative.dev/serving/pkg/reconciler/service"
	resourcenames "knative.dev/serving/pkg/reconciler/service/resources/names"
)

// Reconciler implements controller.Reconciler for Service resources.
type Reconciler struct {
	client                    clientset.Interface
	baseReconciler            *servingService.Reconciler
	routeLister               servinglisters.RouteLister
	revisionLister            servinglisters.RevisionLister
	rolloutOrchestratorLister listers.RolloutOrchestratorLister
	podAutoscalerLister       palisters.PodAutoscalerLister
}

// Check that our Reconciler implements ksvcreconciler.Interface
var _ ksvcreconciler.Interface = (*Reconciler)(nil)

// NewReconciler creates the reference to the Reconciler based on servingclientset.Interface, listers.ConfigurationLister,
// listers.RevisionLister and listers.RouteLister.
func NewReconciler(prclient clientset.Interface, client servingclientset.Interface, configurationLister servinglisters.ConfigurationLister,
	revisionLister servinglisters.RevisionLister, routeLister servinglisters.RouteLister,
	rolloutOrchestratorLister listers.RolloutOrchestratorLister,
	podAutoscalerLister palisters.PodAutoscalerLister) *Reconciler {
	return &Reconciler{
		baseReconciler: servingService.NewReconciler(
			client,
			configurationLister,
			revisionLister,
			routeLister,
		),
		client:                    prclient,
		routeLister:               routeLister,
		revisionLister:            revisionLister,
		rolloutOrchestratorLister: rolloutOrchestratorLister,
		podAutoscalerLister:       podAutoscalerLister,
	}
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, service *servingv1.Service) pkgreconciler.Event {
	rolloutOrchestrator, err := c.rolloutOrchestrator(ctx, service)
	if err != nil {
		return err
	}
	return c.baseReconciler.ReconcileKind(ctx, TransformService(service, rolloutOrchestrator))
}

func (c *Reconciler) rolloutOrchestrator(ctx context.Context, service *servingv1.Service) (*v1.RolloutOrchestrator, error) {
	recorder := controller.GetEventRecorder(ctx)
	routeName := resourcenames.Route(service)
	route, err := c.routeLister.Routes(service.Namespace).Get(routeName)
	if apierrs.IsNotFound(err) {
		route = nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to get the route: %w", err)
	}

	roName := resourcenames.Configuration(service)
	rolloutOrchestrator, err := c.rolloutOrchestratorLister.RolloutOrchestrators(service.Namespace).Get(roName)
	if apierrs.IsNotFound(err) {
		rolloutOrchestrator, err = c.createUpdateRolloutOrchestrator(ctx, service, route, nil, true)
		if err != nil {
			recorder.Eventf(service, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create RolloutOrchestrator %q: %v", roName, err)
			return nil, fmt.Errorf("failed to create RolloutOrchestrator: %w", err)
		}
		recorder.Eventf(service, corev1.EventTypeNormal, "Created", "Created RolloutOrchestrator %q", roName)
	} else if err != nil {
		return nil, fmt.Errorf("failed to get RolloutOrchestrator: %w", err)
	} else if !metav1.IsControlledBy(rolloutOrchestrator, service) {
		// TODO Surface an error in the service's status, and return an error.
		return nil, fmt.Errorf("service: %q does not own the RolloutOrchestrator: %q", service.Name, roName)
	} else if rolloutOrchestrator, err = c.reconcileRolloutOrchestrator(ctx, service, route, rolloutOrchestrator); err != nil {
		return nil, fmt.Errorf("failed to reconcile RolloutOrchestrator: %w", err)
	}

	return rolloutOrchestrator, nil
}

func (c *Reconciler) createUpdateRolloutOrchestrator(ctx context.Context, service *servingv1.Service,
	route *servingv1.Route, so *v1.RolloutOrchestrator, create bool) (*v1.RolloutOrchestrator, error) {
	// To create the RolloutOrchestrator we need to make sure we have stageTraffic and Traffic in the spec, and
	// stageReady, and Ready in the status.
	records := map[string]resources.RevisionRecord{}

	lister := c.revisionLister.Revisions(service.Namespace)
	list, err := lister.List(labels.SelectorFromSet(labels.Set{
		serving.ConfigurationLabelKey: service.Name,
		serving.ServiceLabelKey:       service.Name,
	}))

	logger := logging.FromContext(ctx)

	if err == nil && len(list) > 0 {
		for _, revision := range list {
			record := resources.RevisionRecord{}

			if val, ok := revision.Annotations[autoscaling.MinScaleAnnotationKey]; ok {
				i, err := strconv.ParseInt(val, 10, 32)
				if err == nil {
					record.MinScale = ptr.Int32(int32(i))
				}
			}

			if val, ok := revision.Annotations[autoscaling.MaxScaleAnnotationKey]; ok {
				i, err := strconv.ParseInt(val, 10, 32)
				if err == nil {
					record.MaxScale = ptr.Int32(int32(i))
				}
			}
			record.Name = revision.Name
			records[revision.Name] = record

		}
	}

	so = resources.MakeServiceOrchestrator(service, route, records, logger, so)
	if create {
		so, err = c.client.ServingV1().RolloutOrchestrators(service.Namespace).Create(
			ctx, so, metav1.CreateOptions{})
		if err != nil {
			return so, err
		}
	} else {
		so, err = c.client.ServingV1().RolloutOrchestrators(service.Namespace).Update(ctx, so, metav1.UpdateOptions{})
		if err != nil {
			return so, err
		}
	}
	so, _ = c.calculateStageRevisionTarget(ctx, so)
	origin := so.DeepCopy()
	if equality.Semantic.DeepEqual(origin.Spec, so.Spec) {
		return so, nil
	}
	so, err = c.client.ServingV1().RolloutOrchestrators(service.Namespace).Update(ctx, so, metav1.UpdateOptions{})
	return so, err
}

func (c *Reconciler) calculateStageRevisionTarget(ctx context.Context, so *v1.RolloutOrchestrator) (*v1.RolloutOrchestrator, error) {
	if so.Status.StageRevisionStatus == nil || len(so.Status.StageRevisionStatus) == 0 ||
		so.Spec.InitialRevisions == nil || len(so.Spec.InitialRevisions) == 0 {

		// There is no stage revision status, which indicates that no route is configured. We can directly set
		// the ultimate revision target as the current stage revision target.
		so.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)

	} else {
		if len(so.Spec.InitialRevisions) > 2 || len(so.Spec.TargetRevisions) > 1 {
			// If the initial revision status contains more than one revision, or the ultimate revision target contains
			// more than one revision, we will set the current stage target to the ultimate revision target.
			so.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
			//} else if len(so.Spec.InitialRevisionStatus) == 2 {
			//	// TODO this is a special case
			//	if *so.Spec.InitialRevisionStatus[0].Percent != int64(100) || *so.Spec.InitialRevisionStatus[0].Percent != int64(0) {
			//		so.Spec.StageRevisionTarget = append([]v1.RevisionTarget{}, so.Spec.RevisionTarget...)
			//	}

		} else {
			if so.Spec.StageTargetRevisions == nil {
				// If so.Spec.StageRevisionTarget is not empty, we need to calculate the stage revision target.
				so = c.updateStageRevisionSpec(so)
				return so, nil
			}
			// If the initial revision status and ultimate revision target both contains only one revision, we will
			// roll out the revision incrementally.
			// Check if stage revision status is ready or in progress
			if so.IsStageReady() {
				if so.IsReady() {
					// If the last stage has rolled out, nothing changes.
					return so, nil
				} else {
					// The current stage revision is complete. We need to calculate the next stage target.
					so = c.updateStageRevisionSpec(so)
				}
			} else if so.IsStageInProgress() {
				// Do nothing, because it is in progress to the current so.Spec.StageRevisionTarget
				// so.Spec.StageRevisionTarget is not empty.
				return so, nil
			}
		}
	}

	return so, nil
}

func (c *Reconciler) updateStageRevisionSpec(so *v1.RolloutOrchestrator) *v1.RolloutOrchestrator {
	if len(so.Status.StageRevisionStatus) > 2 || len(so.Spec.TargetRevisions) != 1 {
		return so
	}
	finalRevision := so.Spec.TargetRevisions[0].RevisionName

	startRevisionStatus := so.Status.StageRevisionStatus
	if so.Spec.StageTargetRevisions == nil {
		startRevisionStatus = so.Spec.InitialRevisions
	}
	ratio := resources.OverSubRatio
	found := false
	index := -1
	if len(startRevisionStatus) == 2 {
		if startRevisionStatus[0].RevisionName == finalRevision {
			found = true
			index = 0
			//originIndex = 1
		}
		if startRevisionStatus[1].RevisionName == finalRevision {
			found = true
			index = 1
			//originIndex = 0
		}
		if !found {
			so.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
			return so
		}

		currentTraffic := *startRevisionStatus[index].Percent

		//	finalTraffic := *so.Spec.RevisionTarget[0].Percent

		pa, _ := c.podAutoscalerLister.PodAutoscalers(so.Namespace).Get(finalRevision)
		currentReplicas := *pa.Status.DesiredScale

		pa, _ = c.podAutoscalerLister.PodAutoscalers(so.Namespace).Get(finalRevision)
		targetReplicas := int32(32)
		if pa != nil {
			targetReplicas = *pa.Status.DesiredScale
		}
		if targetReplicas < 0 {
			targetReplicas = 0
		}

		min := startRevisionStatus[index].MinScale
		max := startRevisionStatus[index].MaxScale

		stageRevisionTarget := []v1.TargetRevision{}
		if min == nil {
			if max == nil {
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}

			} else {
				maxV := *max
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else if currentReplicas < maxV {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				} else if currentReplicas == maxV {
					// Full load.
					stageRevisionTarget = c.fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}
			}
		} else {
			if max == nil {
				minV := *min
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else if currentReplicas <= minV {
					// Lowest load.
					stageRevisionTarget = c.lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)

				} else if currentReplicas > minV {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}

			} else {
				minV := *min
				maxV := *max
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else if currentReplicas > minV && currentReplicas < maxV {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				} else if currentReplicas == maxV {
					// Full load.
					stageRevisionTarget = c.fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				} else if currentReplicas <= minV {
					// Lowest load.
					stageRevisionTarget = c.lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}

			}
		}

		so.Spec.StageTargetRevisions = stageRevisionTarget
		t := time.Now()
		so.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(t.Add(time.Minute * 2))
	}

	if len(startRevisionStatus) == 1 {
		if startRevisionStatus[0].RevisionName == finalRevision {
			so.Spec.StageTargetRevisions = so.Spec.TargetRevisions
			return so
		}

		min := startRevisionStatus[0].MinScale
		max := startRevisionStatus[0].MaxScale
		index = 0
		pa, _ := c.podAutoscalerLister.PodAutoscalers(so.Namespace).Get(startRevisionStatus[0].RevisionName)
		currentReplicas := *pa.Status.DesiredScale

		pa, _ = c.podAutoscalerLister.PodAutoscalers(so.Namespace).Get(finalRevision)

		targetReplicas := int32(32)
		if pa != nil {
			targetReplicas = *pa.Status.DesiredScale
		}
		if targetReplicas < 0 {
			targetReplicas = 0
		}

		currentTraffic := *startRevisionStatus[0].Percent

		//	finalTraffic := *so.Spec.RevisionTarget[0].Percent

		stageRevisionTarget := []v1.TargetRevision{}
		if min == nil {
			if max == nil {
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}

			} else {
				maxV := *max
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else if currentReplicas < maxV {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				} else if currentReplicas == maxV {
					// Full load.
					stageRevisionTarget = c.fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}
			}
		} else {
			if max == nil {
				minV := *min
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else if currentReplicas <= minV {
					// Lowest load.
					stageRevisionTarget = c.lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)

				} else if currentReplicas > minV {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}

			} else {
				minV := *min
				maxV := *max
				if currentReplicas == 0 {
					// No traffic, set the stage revision target to final revision target.
					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
				} else if currentReplicas > minV && currentReplicas < maxV {
					// Driven by traffic
					stageRevisionTarget = c.trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				} else if currentReplicas == maxV {
					// Full load.
					stageRevisionTarget = c.fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				} else if currentReplicas == minV {
					// Lowest load.
					stageRevisionTarget = c.lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio)
				}

			}
		}

		//stageRevisionTarget.
		so.Spec.StageTargetRevisions = stageRevisionTarget
		t := time.Now()

		so.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(t.Add(time.Minute * 2))
	}

	return so
}

func (c *Reconciler) reconcileRolloutOrchestrator(ctx context.Context, service *servingv1.Service,
	route *servingv1.Route, so *v1.RolloutOrchestrator) (*v1.RolloutOrchestrator, error) {
	so1, err := c.createUpdateRolloutOrchestrator(ctx, service, route, so, false)
	if err != nil {
		return so, err
	}
	if equality.Semantic.DeepEqual(so.Spec, so1.Spec) {
		return so, nil
	}
	return c.client.ServingV1().RolloutOrchestrators(service.Namespace).Update(ctx, so1, metav1.UpdateOptions{})
}

func (c *Reconciler) trafficDriven(rt []v1.TargetRevision, index int, rtF []v1.TargetRevision,
	currentTraffic int64, namespace string, currentReplicas, targetReplicas int32, ratio int) []v1.TargetRevision {
	return c.lowestLoad(rt, index, rtF, currentTraffic, namespace, currentReplicas, targetReplicas, ratio)
}

func (c *Reconciler) fullLoad(rt []v1.TargetRevision, index int, rtF []v1.TargetRevision,
	currentTraffic int64, namespace string, currentReplicas, targetReplicas int32, ratio int) []v1.TargetRevision {

	return c.lowestLoad(rt, index, rtF, currentTraffic, namespace, currentReplicas, targetReplicas, ratio)
}

func getReplicasTraffic(percent int64, currentReplicas int32, ratio int) (int32, int64) {
	stageReplicas := math.Ceil(float64(int(currentReplicas)) * float64(ratio) / float64((int(percent))))

	stageTrafficDelta := math.Ceil(stageReplicas * float64((int(percent))) / float64(int(currentReplicas)))

	return int32(stageReplicas), int64(stageTrafficDelta)
}

func (c *Reconciler) lowestLoad(rt []v1.TargetRevision, index int, rtF []v1.TargetRevision,
	currentTraffic int64, namespace string, currentReplicas, targetReplicas int32, ratio int) []v1.TargetRevision {
	stageReplicasInt, stageTrafficDeltaInt := getReplicasTraffic(*rt[index].Percent, currentReplicas, ratio)
	var stageRevisionTarget []v1.TargetRevision
	if len(rt) == 1 {
		stageRevisionTarget = make([]v1.TargetRevision, 2, 2)
		if stageTrafficDeltaInt >= 100 {
			stageRevisionTarget = append(rtF, []v1.TargetRevision{}...)
			target := v1.TargetRevision{}
			target.RevisionName = rt[0].RevisionName
			target.MaxScale = rt[0].MaxScale
			target.MinScale = rt[0].MinScale
			target.Direction = "down"
			target.Percent = ptr.Int64(0)
			target.TargetReplicas = ptr.Int32(0)
			stageRevisionTarget = append(stageRevisionTarget, target)
			return stageRevisionTarget
		}

		targetNewRollout := v1.TargetRevision{}
		targetNewRollout.RevisionName = rtF[0].RevisionName
		targetNewRollout.IsLatestRevision = ptr.Bool(true)
		targetNewRollout.MinScale = rtF[0].MinScale
		targetNewRollout.MaxScale = rtF[0].MaxScale
		targetNewRollout.Direction = "up"
		targetNewRollout.TargetReplicas = ptr.Int32(stageReplicasInt)
		targetNewRollout.Percent = ptr.Int64(stageTrafficDeltaInt)
		stageRevisionTarget[1] = targetNewRollout

		target := v1.TargetRevision{}
		target.RevisionName = rt[0].RevisionName
		target.IsLatestRevision = ptr.Bool(false)
		target.MinScale = rt[0].MinScale
		target.MaxScale = rt[0].MaxScale
		target.Direction = "down"
		target.TargetReplicas = ptr.Int32(currentReplicas - stageReplicasInt)
		target.Percent = ptr.Int64(currentTraffic - stageTrafficDeltaInt)
		stageRevisionTarget[0] = target

	} else if len(rt) == 2 {
		stageRevisionTarget = make([]v1.TargetRevision, 0, 2)
		for i, r := range rt {
			if i == index {
				nu := *r.Percent + stageTrafficDeltaInt
				if nu >= 100 {
					fmt.Println("up over 100")
					stageRevisionTarget = append(stageRevisionTarget, rtF...)
					//target := v1.RevisionTarget{}
					//target.TargetReplicas = ptr.Int32(0)
					//stageRevisionTarget = append(stageRevisionTarget, target)
					//return stageRevisionTarget
					fmt.Println(stageRevisionTarget)
				} else {
					target := v1.TargetRevision{}
					target.RevisionName = r.RevisionName
					target.IsLatestRevision = ptr.Bool(true)
					target.MinScale = r.MinScale
					target.MaxScale = r.MaxScale
					target.Direction = "up"
					target.TargetReplicas = ptr.Int32(targetReplicas + stageReplicasInt)
					target.Percent = ptr.Int64(*r.Percent + stageTrafficDeltaInt)
					stageRevisionTarget = append(stageRevisionTarget, target)
				}

			} else {
				pa, _ := c.podAutoscalerLister.PodAutoscalers(namespace).Get(r.RevisionName)
				oldReplicas := int32(0)
				if pa != nil {
					oldReplicas = *pa.Status.DesiredScale
				}
				if oldReplicas < 0 {
					oldReplicas = 0
				}

				if *r.Percent-stageTrafficDeltaInt <= 0 {
					target := v1.TargetRevision{}
					target.RevisionName = r.RevisionName
					target.IsLatestRevision = ptr.Bool(false)
					target.MinScale = r.MinScale
					target.MaxScale = r.MaxScale
					target.Direction = "down"
					target.TargetReplicas = ptr.Int32(0)
					target.Percent = ptr.Int64(0)
					stageRevisionTarget = append(stageRevisionTarget, target)
					fmt.Println("down below 0")
					fmt.Println(stageRevisionTarget)
				} else {
					target := v1.TargetRevision{}
					target.RevisionName = r.RevisionName
					target.IsLatestRevision = ptr.Bool(false)
					target.MinScale = r.MinScale
					target.MaxScale = r.MaxScale
					target.Direction = "down"
					if oldReplicas-stageReplicasInt <= 0 {
						target.TargetReplicas = r.TargetReplicas
					} else {
						target.TargetReplicas = ptr.Int32(oldReplicas - stageReplicasInt)
					}
					if *r.Percent-stageTrafficDeltaInt <= 0 {
						target.Percent = ptr.Int64(0)
					} else {
						target.Percent = ptr.Int64(*r.Percent - stageTrafficDeltaInt)
					}

					stageRevisionTarget = append(stageRevisionTarget, target)
					fmt.Println("down not below 0")
					fmt.Println(stageRevisionTarget)
				}

			}
		}

	}

	return stageRevisionTarget
}

func TransformService(service *servingv1.Service, so *v1.RolloutOrchestrator) *servingv1.Service {
	// TODO Keep the the service the same. We will implement later.
	return service
}
