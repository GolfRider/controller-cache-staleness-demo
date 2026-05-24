// watch-proxy: the Day-2 STRETCH CAPSTONE.
//
// WHY THIS EXISTS:
// The LagInjector simulates the *effect* of staleness (deterministic, clean,
// but you carry the "it's faked" caveat). This proxy induces the *real* cause:
// it sits between the controller and the API server and adds latency to WATCH
// responses — the stream that feeds the informer. With watch delivery delayed,
// the informer cache genuinely falls behind etcd, exactly as it does under
// real API-server pressure (the GKE 745-pods/sec scenario).
//
// STAGE NARRATIVE (the progression that kills the "edge case" objection):
//   1. LagInjector  -> "mechanism, simulated, perfectly clean"
//   2. THIS proxy   -> "induced the way production induces it: slow watch path"
//   3. burst.sh     -> "and here's the real churn storm that creates that latency"
//
// STATUS: stub. ~80-120 lines to finish. Point the controller's kubeconfig at
// this proxy instead of the API server; the controller code does NOT change —
// it just reads a cache that is now really stale.
//
// IMPLEMENTATION SKETCH:
//   - httputil.NewSingleHostReverseProxy(apiServerURL)
//   - detect watch requests: query param ?watch=true (and/or verb on the path)
//   - on watch responses, wrap the response body so each streamed event is
//     delayed by a configurable duration before flush (introduces watch lag)
//   - expose the same /lag control endpoint to tune delay live
//   - TLS: either terminate with the cluster CA + a client cert, or run against
//     `kubectl proxy` upstream to dodge TLS for the demo
//
// CAVEAT TO STATE ON STAGE: this delays watch delivery to *simulate* API-server
// pressure. It reproduces the real condition (slow watch -> stale cache) rather
// than faking the symptom — a categorically stronger position than the injector
// alone, but still a controlled stand-in for a loaded production control plane.
package main

func main() {
	panic("watch-proxy: stretch capstone, not yet implemented — see file header for the sketch")
}
