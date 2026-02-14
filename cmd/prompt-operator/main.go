// Command prompt-operator runs a Kubernetes controller that syncs Prompt CRs to a loom registry.
package main

import (
	"flag"
	"os"

	"github.com/klejdi94/loom/k8s"
	"github.com/klejdi94/loom/k8s/api/v1"
	"github.com/klejdi94/loom/registry"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		os.Exit(1)
	}
	reg := registry.NewMemoryRegistry()
	reconciler := &k8s.PromptReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Registry: reg,
	}
	if err = reconciler.SetupWithManager(mgr); err != nil {
		os.Exit(1)
	}
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		os.Exit(1)
	}
}
