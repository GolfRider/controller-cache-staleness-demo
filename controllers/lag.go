// lag.go — the microscope's focus knob.
//
// HONESTY NOTE (read on stage, essentially verbatim):
// This injects SIMULATED cache staleness so the demo is deterministic and
// reproducible. It is NOT real DeltaFIFO backlog. Real backlog (hack/burst.sh
// floods the cache with a node-kill storm) produces the same effect
// non-deterministically; we use simulation here so the failure reproduces every
// time on stage. The phenomenon is real (KCM #130767, zookeeper-operator #314);
// the knob is a teaching device.
//
// WHAT IT MODELS:
// "The controller can't yet see its N most-recent writes." We track ground
// truth (how many pods really exist) separately from the controller's view
// (truth minus the lag window). The gap = staleness.
package controllers

import (
	"sync"
	"time"
)

// LagInjector holds, per-workload, the ground-truth pod count and how many
// recent writes are currently "hidden" from the controller's cache view.
type LagInjector struct {
	mu sync.Mutex

	// window is how many of the most-recent creates are hidden from the read
	// view. window == 0 means no lag (cache fresh). Set via SetWindow.
	window int

	// trueCounts[name] = how many pods the harness has actually created for
	// this workload (ground truth). Advanced by RecordCreate.
	trueCounts map[string]int

	// lagDuration is informational: the simulated age of the photo, surfaced
	// as a metric in seconds so Grafana can plot "how stale."
	lagDuration time.Duration
}

func NewLagInjector() *LagInjector {
	return &LagInjector{trueCounts: map[string]int{}}
}

// SetWindow sets how many recent writes are hidden (the staleness depth).
// hack/inject-lag.sh calls an HTTP endpoint that flips this at demo time.
func (li *LagInjector) SetWindow(n int, d time.Duration) {
	li.mu.Lock()
	defer li.mu.Unlock()
	li.window = n
	li.lagDuration = d
}

// RecordCreate advances ground truth when the controller really creates a pod.
func (li *LagInjector) RecordCreate(name string) {
	li.mu.Lock()
	defer li.mu.Unlock()
	li.trueCounts[name]++
}

// TrueCount returns ground truth. We pass the cache's reported len as a floor in
// case the harness restarted; max of the two is the safe truth.
func (li *LagInjector) TrueCount(name string, cacheLen int) int {
	li.mu.Lock()
	defer li.mu.Unlock()
	if li.trueCounts[name] < cacheLen {
		li.trueCounts[name] = cacheLen
	}
	return li.trueCounts[name]
}

// ApplyToCount takes the cache's reported pod count and hides the most-recent
// `window` writes, returning the STALE view the controller will act on.
//
// This is the single line that turns a fresh read into a stale one.
func (li *LagInjector) ApplyToCount(name string, cacheLen int) int {
	li.mu.Lock()
	defer li.mu.Unlock()
	observed := cacheLen - li.window
	if observed < 0 {
		observed = 0
	}
	return observed
}

// IsStale reports whether the cache has NOT yet caught up to our recent writes,
// i.e. whether `window` recent writes are currently hidden. This is the
// read-side mirror of the gate: in production it is
// `informer.LastStoreSyncResourceVersion() < lastWriteRV`. Here, any nonzero
// lag window means the cache is behind our writes.
func (li *LagInjector) IsStale(name string, cacheLen int) bool {
	li.mu.Lock()
	defer li.mu.Unlock()
	return li.window > 0
}

// LagSeconds surfaces the simulated photo-age for the metric.
func (li *LagInjector) LagSeconds() float64 {
	li.mu.Lock()
	defer li.mu.Unlock()
	return li.lagDuration.Seconds()
}
