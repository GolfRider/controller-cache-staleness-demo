// main.go — wires the manager and picks the controller by --mode.
//
// Run two instances during the demo (different --metrics-bind-address), one
// --mode=naive and one --mode=gated, against the same cluster + same workload +
// same lag injection, and compare their metrics side by side in Grafana.
package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	demov1 "github.com/GolfRider/staleness-demo/api/v1alpha1"
	"github.com/GolfRider/staleness-demo/controllers"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = demov1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
}

func main() {
	var mode, metricsAddr, lagAddr string
	flag.StringVar(&mode, "mode", "naive", "controller mode: naive | gated")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "metrics endpoint")
	flag.StringVar(&lagAddr, "lag-control-address", ":9090", "HTTP endpoint to flip lag at demo time")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: metricsAddr},
	})
	if err != nil {
		panic(err)
	}

	lag := controllers.NewLagInjector()

	switch mode {
	case "gated":
		if err := (&controllers.GatedReconciler{Client: mgr.GetClient(), Lag: lag}).SetupWithManager(mgr); err != nil {
			panic(err)
		}
	default:
		if err := (&controllers.NaiveReconciler{Client: mgr.GetClient(), Lag: lag}).SetupWithManager(mgr); err != nil {
			panic(err)
		}
	}

	// Lag-control endpoint: POST /lag {"window":N,"seconds":S} to inject staleness
	// live during the demo. hack/inject-lag.sh calls this.
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/lag", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Window  int     `json:"window"`
				Seconds float64 `json:"seconds"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			lag.SetWindow(body.Window, time.Duration(body.Seconds*float64(time.Second)))
			w.WriteHeader(http.StatusOK)
		})
		_ = http.ListenAndServe(lagAddr, mux)
	}()

	_ = mgr.AddHealthzCheck("healthz", healthz.Ping)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		panic(err)
	}
}
