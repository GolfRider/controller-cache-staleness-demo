// naive_controller.go — the controller that BREAKS.
//
// MENTAL MODEL:
// This controller makes the assumption every controller author makes: "my cache
// reflects reality." It does write-read-decide with NO gate. Under cache lag,
// its READ returns a stale child-pod count, and its DECIDE acts on that stale
// count — creating duplicate pods or deleting pods that shouldn't be deleted.
//
// We instrument three signals (see metrics.go):
//   - cacheLagGauge      (REAL-ish: how far behind the read view is)
//   - wrongActionsTotal  (SYNTHETIC/harness: actions that contradict ground truth)
//   - reconcileTotal     (context)
//
// The wrong-action measurement is only possible because THE HARNESS KNOWS GROUND
// TRUTH: the desired replica count is in spec, and the harness tracks the true
// number of pods it has created. We compare the controller's decision against
// that truth. In a real cluster you cannot measure this directly — which is
// exactly why KEP-5647 measures LAG (the cause) instead of wrong actions (the
// effect). Say this on stage.
package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	demov1 "github.com/GolfRider/staleness-demo/api/v1alpha1"
)

// NaiveReconciler reconciles a FakeWorkload with no staleness discipline.
type NaiveReconciler struct {
	client.Client

	// Lag is the injected, SIMULATED staleness applied to the cache read.
	// When Lag > 0, the controller's view of child pods is artificially held
	// behind reality by roughly this duration's worth of recent writes. This is
	// a teaching knob: it makes a probabilistic race deterministic so the demo
	// is reproducible on stage. Label it "simulated lag" — it is not real
	// DeltaFIFO backlog (though hack/burst.sh can produce that too).
	Lag *LagInjector
}

// Reconcile implements the write-read-decide loop.
func (r *NaiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	reconcileTotal.WithLabelValues("naive").Inc()

	// --- Fetch the FakeWorkload (the object we own) ---
	var fw demov1.FakeWorkload
	if err := r.Get(ctx, req.NamespacedName, &fw); err != nil {
		// Ignore not-found: object was deleted, nothing to do.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	desired := int(fw.Spec.Replicas)

	// --- READ (step 3): list the child pods we own, FROM CACHE ---
	// This is the line that goes stale. r.List reads the informer cache, not
	// etcd. Under injected lag, the LagInjector hides recently-created pods from
	// this view, so observed < truth.
	var pods corev1.PodList
	listOpts := []client.ListOption{
		client.InNamespace(fw.Namespace),
		client.MatchingLabels(fw.ChildPodLabels()),
	}
	if err := r.List(ctx, &pods, listOpts...); err != nil {
		return ctrl.Result{}, err
	}

	// Apply simulated staleness: drop the most-recently-created pods from the
	// view so the controller "doesn't see" its own recent writes.
	observed := r.Lag.ApplyToCount(fw.Name, len(pods.Items))

	// Record the gap between what we observed and the true pod count.
	truth := r.Lag.TrueCount(fw.Name, len(pods.Items))
	cacheLagGauge.WithLabelValues("naive", fw.Name).Set(float64(truth - observed))

	l.V(1).Info("naive reconcile", "desired", desired, "observed", observed, "truth", truth)

	// --- DECIDE (step 4): act on the (possibly stale) observed count ---
	switch {
	case observed < desired:
		// We think we have too few -> create the difference.
		// BUG: if observed is stale-low, we create duplicates we don't need.
		toCreate := desired - observed
		for i := 0; i < toCreate; i++ {
			if err := r.createChildPod(ctx, &fw); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Ground-truth check: were these creates actually warranted?
		// If truth was already >= desired, every create here is a WRONG ACTION.
		if truth >= desired {
			wrong := minInt(toCreate, truth-desired+toCreate)
			wrongActionsTotal.WithLabelValues("naive", "over_create").Add(float64(wrong))
			l.Info("WRONG ACTION: created pods that weren't needed (stale read)",
				"count", wrong, "observed", observed, "truth", truth, "desired", desired)
		}

	case observed > desired:
		// We think we have too many -> delete the excess.
		toDelete := observed - desired
		_ = toDelete // deletion path symmetric; omitted for brevity in v1
	}

	// --- WRITE-BACK: record our (stale) belief into status ---
	fw.Status.ObservedReplicas = int32(observed)
	fw.Status.ObservedGeneration = fw.Generation
	if err := r.Status().Update(ctx, &fw); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		return ctrl.Result{}, err
	}

	// Requeue periodically so the loop keeps running during the demo.
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *NaiveReconciler) createChildPod(ctx context.Context, fw *demov1.FakeWorkload) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fw.Name + "-",
			Namespace:    fw.Namespace,
			Labels:       fw.ChildPodLabels(),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "pause",
				Image: "registry.k8s.io/pause:3.9",
			}},
		},
	}
	// Set owner ref so pods are GC'd with the FakeWorkload.
	if err := ctrl.SetControllerReference(fw, pod, r.Scheme()); err != nil {
		return err
	}
	if err := r.Create(ctx, pod); err != nil {
		return fmt.Errorf("create child pod: %w", err)
	}
	// Tell the harness a real write happened (advances ground truth).
	r.Lag.RecordCreate(fw.Name)
	return nil
}

// SetupWithManager wires the controller to watch FakeWorkloads and owned Pods.
func (r *NaiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Pre-initialize series to 0 so the panel renders a line from the start
	// (naive's wrong-actions starts at 0, then climbs under lag).
	wrongActionsTotal.WithLabelValues("naive", "over_create").Add(0)
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1.FakeWorkload{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = types.NamespacedName{}
