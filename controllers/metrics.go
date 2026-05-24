// metrics.go — the three signals that tell the whole story.
//
//	cacheLagGauge     CAUSE        : how far the read view is behind truth
//	staleSkipsTotal   INTERVENTION : times the gate refused to act on a stale read
//	wrongActionsTotal EFFECT       : actions contradicting ground truth (SYNTHETIC/harness)
//	reconcileTotal    CONTEXT      : reconcile volume
//
// Naive controller: lag climbs, NO skips (no gate), wrong-actions climb.
// Gated controller: lag climbs, skips fire, wrong-actions stay flat.
// The gap between the two wrong-action lines is the money slide; the skip line
// explains why the gap exists.
package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	cacheLagGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "staleness_demo_cache_lag",
		Help: "Observed gap between true child-pod count and the (stale) cached view. >0 means acting on a stale photo.",
	}, []string{"controller", "workload"})

	staleSkipsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "staleness_demo_stale_skips_total",
		Help: "Reconciles the gate refused because the cache had not caught up to the controller's last write (resourceVersion fencing).",
	}, []string{"controller"})

	wrongActionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "staleness_demo_wrong_actions_total",
		Help: "SYNTHETIC/harness-only: actions taken that contradict ground truth. Not a real K8s metric; only measurable because the harness knows desired truth.",
	}, []string{"controller", "kind"})

	reconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "staleness_demo_reconcile_total",
		Help: "Total reconciles, by controller.",
	}, []string{"controller"})
)

func init() {
	metrics.Registry.MustRegister(cacheLagGauge, staleSkipsTotal, wrongActionsTotal, reconcileTotal)
}
