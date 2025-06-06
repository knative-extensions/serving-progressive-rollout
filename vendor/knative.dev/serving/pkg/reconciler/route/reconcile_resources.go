/*
Copyright 2018 The Knative Authors

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

package route

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"

	"knative.dev/networking/pkg/apis/networking"
	netv1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmap"
	"knative.dev/pkg/logging"
	"knative.dev/serving/pkg/apis/serving"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/reconciler/route/config"
	"knative.dev/serving/pkg/reconciler/route/resources"
	"knative.dev/serving/pkg/reconciler/route/resources/names"
	"knative.dev/serving/pkg/reconciler/route/traffic"
)

func (c *Reconciler) reconcileIngress(
	ctx context.Context, r *v1.Route, tc *traffic.Config,
	tls []netv1alpha1.IngressTLS,
	ingressClass string,
	acmeChallenges ...netv1alpha1.HTTP01Challenge,
) (*netv1alpha1.Ingress, *traffic.Rollout, error) {
	recorder := controller.GetEventRecorder(ctx)
	var effectiveRO *traffic.Rollout

	ingress, err := c.ingressLister.Ingresses(r.Namespace).Get(names.Ingress(r))
	if apierrs.IsNotFound(err) {
		desired, err := resources.MakeIngress(ctx, r, tc, tls, ingressClass, acmeChallenges...)
		if err != nil {
			return nil, nil, err
		}
		ingress, err = c.netclient.NetworkingV1alpha1().Ingresses(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			recorder.Eventf(r, corev1.EventTypeWarning, "CreationFailed", "Failed to create Ingress: %v", err)
			return nil, nil, fmt.Errorf("failed to create Ingress: %w", err)
		}

		recorder.Eventf(r, corev1.EventTypeNormal, "Created", "Created Ingress %q", ingress.GetName())
		return ingress, tc.BuildRollout(), nil
	} else if err != nil {
		return nil, nil, err
	} else {
		// Ingress exists. We need to compute the rollout spec diff.
		effectiveRO = c.reconcileRollout(ctx, r, tc, ingress)
		desired, err := resources.MakeIngressWithRollout(ctx, r, tc, effectiveRO,
			tls, ingressClass, acmeChallenges...)
		if err != nil {
			return nil, nil, err
		}

		if !equality.Semantic.DeepEqual(ingress.Spec, desired.Spec) ||
			!equality.Semantic.DeepEqual(ingress.Annotations, desired.Annotations) ||
			!equality.Semantic.DeepEqual(ingress.Labels, desired.Labels) {
			// It is notable that one reason for differences here may be defaulting.
			// When that is the case, the Update will end up being a nop because the
			// webhook will bring them into alignment and no new reconciliation will occur.
			// Also, compare annotation and label in case ingress.Class or parent route's labels
			// is updated.

			// Don't modify the informers copy.
			origin := ingress.DeepCopy()
			origin.Spec = desired.Spec
			origin.Annotations = desired.Annotations
			origin.Labels = desired.Labels

			updated, err := c.netclient.NetworkingV1alpha1().Ingresses(origin.Namespace).Update(
				ctx, origin, metav1.UpdateOptions{})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to update Ingress: %w", err)
			}
			return updated, effectiveRO, nil
		}
	}

	return ingress, effectiveRO, err
}

func (c *Reconciler) deleteOrphanedServices(ctx context.Context, r *v1.Route, activeServices []resources.ServicePair) error {
	ns := r.Namespace

	active := make(sets.Set[string], len(activeServices))

	for _, service := range activeServices {
		active.Insert(service.Service.Name)
	}

	routeLabelSelector := labels.SelectorFromSet(labels.Set{serving.RouteLabelKey: r.Name})
	allServices, err := c.serviceLister.Services(ns).List(routeLabelSelector)
	if err != nil {
		return fmt.Errorf("failed to fetch existing services: %w", err)
	}

	for _, service := range allServices {
		if active.Has(service.Name) {
			continue
		}

		if err := c.kubeclient.CoreV1().Services(ns).Delete(ctx, service.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete Service: %w", err)
		}

		// Delete the endpoint if it exists
		_, err := c.endpointsLister.Endpoints(ns).Get(service.Name)
		if apierrs.IsNotFound(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to list endpoint: %w", err)
		}

		if err := c.kubeclient.CoreV1().Endpoints(ns).Delete(ctx, service.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete Endpoints: %w", err)
		}
	}

	return nil
}

func (c *Reconciler) reconcilePlaceholderServices(ctx context.Context, route *v1.Route, targets map[string]traffic.RevisionTargets) ([]resources.ServicePair, error) {
	logger := logging.FromContext(ctx)
	recorder := controller.GetEventRecorder(ctx)
	ns := route.Namespace
	services := make([]resources.ServicePair, 0, len(targets))
	names := make(sets.Set[string], len(targets))

	// Note: this is done in order for the tests to be
	// deterministic since they assert creations in order
	for name := range targets {
		names.Insert(name)
	}

	for _, name := range sets.List(names) {
		desiredService, err := resources.MakeK8sPlaceholderService(ctx, route, name)
		if err != nil {
			return nil, fmt.Errorf("failed to construct placeholder k8s service: %w", err)
		}

		service, err := c.serviceLister.Services(ns).Get(desiredService.Name)
		if apierrs.IsNotFound(err) {
			// Doesn't exist, create it.
			service, err = c.kubeclient.CoreV1().Services(ns).Create(ctx, desiredService, metav1.CreateOptions{})
			if err != nil {
				recorder.Eventf(route, corev1.EventTypeWarning, "CreationFailed",
					"Failed to create placeholder service %q: %v", desiredService.Name, err)
				return nil, fmt.Errorf("failed to create placeholder service: %w", err)
			}
			logger.Info("Created service ", desiredService.Name)
			recorder.Eventf(route, corev1.EventTypeNormal, "Created", "Created placeholder service %q", desiredService.Name)
		} else if err != nil {
			return nil, err
		} else if !metav1.IsControlledBy(service, route) {
			// Surface an error in the route's status, and return an error.
			route.Status.MarkServiceNotOwned(desiredService.Name)
			return nil, fmt.Errorf("route: %q does not own Service: %q", route.Name, desiredService.Name)
		}

		// Check if we have endpoints for this service
		endpoints, err := c.endpointsLister.Endpoints(ns).Get(desiredService.Name)
		if apierrs.IsNotFound(err) {
			// noop
		} else if err != nil {
			return nil, err
		} else if !metav1.IsControlledBy(endpoints, route) {
			// Surface an error in the route's status, and return an error.
			route.Status.MarkEndpointNotOwned(desiredService.Name)
			return nil, fmt.Errorf("route: %q does not own Endpoints: %q", route.Name, desiredService.Name)
		}

		services = append(services, resources.ServicePair{
			Service:   service,
			Endpoints: endpoints,
			Tag:       name,
		})
	}

	// Delete any current services that was no longer desired.
	if err := c.deleteOrphanedServices(ctx, route, services); err != nil {
		return nil, err
	}

	// TODO(mattmoor): This is where we'd look at the state of the Service and
	// reflect any necessary state into the Route.
	return services, nil
}

func (c *Reconciler) updatePlaceholderServices(ctx context.Context, route *v1.Route, pairs []resources.ServicePair, ingress *netv1alpha1.Ingress) error {
	logger := logging.FromContext(ctx)
	ns := route.Namespace

	eg, egCtx := errgroup.WithContext(ctx)
	for _, from := range pairs {
		eg.Go(func() error {
			to, err := resources.MakeK8sService(egCtx, route, from.Tag, ingress, resources.IsClusterLocalService(from.Service))
			if err != nil {
				// Loadbalancer not ready, no need to update.
				logger.Warnw("Failed to update k8s service", zap.Error(err))
				return nil
			}

			canUpdate := false

			if from.Spec.Type != to.Spec.Type {
				switch from.Spec.Type {
				// Transitions from ExternalName to any type should work
				case corev1.ServiceTypeExternalName:
					canUpdate = true
				default:
					// Transitions from ClusterIP to ExternalName Fail
					// See: https://github.com/kubernetes/kubernetes/issues/104329
				}
			} else if from.Spec.Type == corev1.ServiceTypeClusterIP {
				if from.Spec.ClusterIP == to.Spec.ClusterIP {
					canUpdate = true
				} else if from.Spec.ClusterIP != corev1.ClusterIPNone && to.Spec.ClusterIP == "" {
					// Copy over the cluster IP
					to.Spec.ClusterIP = from.Spec.ClusterIP
					canUpdate = true
				}
				// else:
				//   clusterIPs are immutable thus any transition requires a recreate
				//   ie. "None" <=> "" (blank - request an IP)
			} else /* types are the same and not clusterIP */ {
				canUpdate = true
			}

			if canUpdate {
				// Make sure that the service has the proper specification.
				if !equality.Semantic.DeepEqual(from.Spec, to.Spec) ||
					!equality.Semantic.DeepEqual(from.Service.Annotations, kmap.Union(from.Service.Annotations, to.Service.Annotations)) ||
					!equality.Semantic.DeepEqual(from.Service.Labels, kmap.Union(from.Service.Labels, to.Service.Labels)) {
					// Don't modify the informers copy.
					existing := from.Service.DeepCopy()
					existing.Spec = to.Service.Spec
					existing.Annotations = kmap.Union(from.Service.Annotations, to.Service.Annotations)
					existing.Labels = kmap.Union(from.Service.Labels, to.Service.Labels)
					if _, err := c.kubeclient.CoreV1().Services(ns).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
						return err
					}
				}
			} else {
				if err := c.kubeclient.CoreV1().Services(ns).Delete(ctx, from.Service.Name, metav1.DeleteOptions{}); err != nil {
					return err
				}
				if _, err := c.kubeclient.CoreV1().Services(ns).Create(ctx, to.Service, metav1.CreateOptions{}); err != nil {
					return err
				}
			}

			if from.Endpoints != nil && to.Endpoints != nil {
				if !equality.Semantic.DeepEqual(from.Endpoints.Subsets, to.Endpoints.Subsets) {
					// Don't modify the informers copy.
					existing := from.Endpoints.DeepCopy()
					existing.Subsets = to.Endpoints.Subsets
					if _, err := c.kubeclient.CoreV1().Endpoints(ns).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
						return err
					}
				}
			} else if from.Endpoints == nil && to.Endpoints == nil {
				// noop
			} else if from.Endpoints == nil {
				if _, err := c.kubeclient.CoreV1().Endpoints(ns).Create(ctx, to.Endpoints, metav1.CreateOptions{}); err != nil {
					return err
				}
			} else if to.Endpoints == nil {
				if err := c.kubeclient.CoreV1().Endpoints(ns).Delete(ctx, from.Endpoints.Name, metav1.DeleteOptions{}); err != nil {
					return err
				}
			}

			return nil
		})
	}

	// TODO(mattmoor): This is where we'd look at the state of the Service and
	// reflect any necessary state into the Route.
	return eg.Wait()
}

func deserializeRollout(ctx context.Context, ro string) *traffic.Rollout {
	if ro == "" {
		return nil
	}
	r := &traffic.Rollout{}
	// Failure can happen if users manually tweaked the
	// annotation or there's etcd corruption. Just log, rollouts
	// are not mission critical.
	if err := json.Unmarshal([]byte(ro), r); err != nil {
		logging.FromContext(ctx).Warnw("Error deserializing Rollout: "+ro,
			zap.Error(err))
		return nil
	}
	if !r.Validate() {
		logging.FromContext(ctx).Warnw("Deserializing Rollout is invalid: " + ro)
		return nil
	}
	return r
}

func (c *Reconciler) reconcileRollout(
	ctx context.Context, r *v1.Route, tc *traffic.Config,
	ingress *netv1alpha1.Ingress,
) *traffic.Rollout {
	cfg := config.FromContext(ctx)

	// Is there rollout duration specified?
	rd := int(r.RolloutDuration().Seconds())
	if rd == 0 {
		// If not, check if there's a cluster-wide default.
		rd = cfg.Network.RolloutDurationSecs
	}
	curRO := tc.BuildRollout()
	// When rollout is disabled just create the baseline annotation.
	if rd <= 0 {
		return curRO
	}
	// Get the current rollout state as described by the traffic.
	logger := logging.FromContext(ctx).Desugar().With(
		zap.Int("durationSecs", rd))
	logger.Debug("Rollout is enabled. Stepping from previous state.")
	// Get the previous rollout state from the annotation.
	// If it's corrupt, inexistent, or otherwise incorrect,
	// the prevRO will be just nil rollout.
	prevRO := deserializeRollout(ctx,
		ingress.Annotations[networking.RolloutAnnotationKey])

	// And recompute the rollout state.
	now := c.clock.Now().UnixNano()

	// Now check if the ingress status changed from not ready to ready.
	rtView := r.Status.GetCondition(v1.RouteConditionIngressReady)
	if prevRO != nil && ingress.IsReady() && !rtView.IsTrue() {
		logger.Debug("Observing Ingress not-ready to ready switch condition for rollout")
		prevRO.ObserveReady(ctx, now, float64(rd))
	}

	effectiveRO, nextStepTime := curRO.Step(ctx, prevRO, now)
	if nextStepTime > 0 {
		nextStepTime -= now
		c.enqueueAfter(r, time.Duration(nextStepTime))
		logger.Debug("Re-enqueuing after", zap.Duration("nextStepTime", time.Duration(nextStepTime)))
	}

	// Comparing and diffing isn't cheap so do it only if we're going
	// to actually log the message.
	// Those are well known types, cmp won't panic.
	if logger.Core().Enabled(zapcore.DebugLevel) && !cmp.Equal(prevRO, effectiveRO) {
		logger.Debug("Rollout diff:(-was,+now)",
			zap.String("diff", cmp.Diff(prevRO, effectiveRO)))
	}
	return effectiveRO
}
