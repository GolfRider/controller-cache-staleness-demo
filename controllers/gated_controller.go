// gated_controller.go — the controller that SURVIVES.
//
// THE ENTIRE TALK IN ONE FILE:
// This is byte-for-byte the naive controller plus ONE discipline: before acting
// on a cache read, it asks "has my cache caught up to my own last write?" If not,
// it refuses to act and requeues. This is resourceVersion fencing — the exact
// mechanism KEP-5647 formalizes (LastStoreSyncResourceVersion + requeue).
//
// THE INSIGHT (say this on stage):
// The gate does NOT make the cache fresher. It makes the controller HONEST about
// when the cache isn't fresh. It converts "act confidently on a stale read" into
// "wait until the read is trustworthy." That's the whole Monday-morning takeaway.
//
// WHY IT'S A SEPARATE FILE, NOT A FLAG IN THE NAIVE ONE:
// So the diff is visible. On stage you show the two Reconcile() funcs side by
// side; the only difference is the gate block. ~10 lines stand between a wrong
// action and a safe requeue.
package controllers

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	demov1 "github.com/GolfRider/staleness-demo/api/v1alpha1"
)

// GatedReconciler is NaiveReconciler + resourceVersion fencing.
type GatedReconciler struct {
	client.Client
	Lag *LagInjector
}

func (r *GatedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	reconcileTotal.WithLabelValues("gated").Inc()

	var fw demov1.FakeWorkload
	if err := r.Get(ctx, req.NamespacedName, &fw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	desired := int(fw.Spec.Replicas)

	// READ from cache (same as naive).
	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(fw.Namespace),
		client.MatchingLabels(fw.ChildPodLabels()),
	); err != nil {
		return ctrl.Result{}, err
	}
	observed := r.Lag.ApplyToCount(fw.Name, len(pods.Items))
	truth := r.Lag.TrueCount(fw.Name, len(pods.Items))
	cacheLagGauge.WithLabelValues("gated", fw.Name).Set(float64(truth - observed))

	// ========================================================================
	// THE GATE — resourceVersion fencing. THIS is the entire difference.
	//
	// "Has my cache caught up to my own last write?" If the cache is still
	// behind what I last wrote, my `observed` count is stale and any decision I
	// make now risks a wrong action. So I refuse to act and requeue with
	// backoff. The cache will catch up; the work item will come back; next time
	// the read is trustworthy.
	//
	// In production with KEP-5647 this check is:
	//     cacheRV := informer.LastStoreSyncResourceVersion()
	//     if cacheRV < r.lastWriteRV[fw.Name] { requeue }
	// KEP-5647 is alpha/opt-in and does not cover custom controllers, so we
	// implement the same shape by hand. The harness exposes the same truth via
	// the lag window: if lag > 0, the cache has not caught up to our writes.
	if r.Lag.IsStale(fw.Name, len(pods.Items)) {
		staleSkipsTotal.WithLabelValues("gated").Inc()
		l.Info("GATE FIRED: cache behind last write, refusing to act, requeueing",
			"observed", observed, "truth", truth, "desired", desired)
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}
	// ========================================================================

	// DECIDE — only reached when the read is trustworthy. Identical to naive.
	switch {
	case observed < desired:
		toCreate := desired - observed
		for i := 0; i < toCreate; i++ {
			if err := r.createChildPod(ctx, &fw); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Because the gate ensured a fresh read, truth==observed here, so these
		// creates are warranted. wrongActionsTotal{gated} stays flat by design.
	case observed > desired:
		// symmetric delete path omitted in v1
	}

	fw.Status.ObservedReplicas = int32(observed)
	fw.Status.ObservedGeneration = fw.Generation
	if err := r.Status().Update(ctx, &fw); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *GatedReconciler) createChildPod(ctx context.Context, fw *demov1.FakeWorkload) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fw.Name + "-",
			Namespace:    fw.Namespace,
			Labels:       fw.ChildPodLabels(),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "pause", Image: "registry.k8s.io/pause:3.9"}},
		},
	}
	if err := ctrl.SetControllerReference(fw, pod, r.Scheme()); err != nil {
		return err
	}
	if err := r.Create(ctx, pod); err != nil {
		return err
	}
	r.Lag.RecordCreate(fw.Name)
	return nil
}

func (r *GatedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Pre-initialize gated's wrong-actions series to 0 so the money panel shows
	// a visible flat-zero line for gated (not an absent series). This is the
	// "gated flat" the panel title promises: it never takes a wrong action.
	wrongActionsTotal.WithLabelValues("gated", "over_create").Add(0)
	staleSkipsTotal.WithLabelValues("gated").Add(0)
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1.FakeWorkload{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
