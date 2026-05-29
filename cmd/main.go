package main

import (
	"flag"
	"net/http"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"diagnostic-operator/internal/config"
	"diagnostic-operator/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "diagnostic-operator.example.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Load diagnostic rules/templates
	rules := config.LoadTemplates()
	setupLog.Info("Loaded diagnostic rules", "count", len(rules))

	// Setup the EventReconciler
	if err = (&controller.EventReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Rules:  rules,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Event")
		os.Exit(1)
	}

	// Add health and readiness probes
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Setup HTTP endpoint for manual event queries
	queryHandler := &controller.QueryHandler{
		KubeconfigPath: os.Getenv("KUBECONFIG"),
	}

	http.HandleFunc("/query-events", queryHandler.HandleEventQuery)
	http.HandleFunc("/query-events-all", queryHandler.HandleAllNamespacesQuery)

	// Start query API server in background
	go func() {
		setupLog.Info("Starting query API server on :8082")
		if err := http.ListenAndServe(":8082", nil); err != nil {
			setupLog.Error(err, "Query API server failed")
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
