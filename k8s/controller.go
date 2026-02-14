// Package k8s provides a Kubernetes controller that syncs Prompt CRs to a loom registry.
package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/klejdi94/loom/core"
	"github.com/klejdi94/loom/k8s/api/v1"
	"github.com/klejdi94/loom/registry"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PromptReconciler reconciles Prompt CRs by storing them in a registry.
type PromptReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry registry.Registry
}

// Reconcile converts the Prompt CR to core.Prompt and stores it in the registry; then updates status.
func (r *PromptReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cr := &v1.Prompt{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	prompt := crToPrompt(cr)
	if prompt.ID == "" {
		prompt.ID = req.Name
	}
	if prompt.Version == "" {
		prompt.Version = "1.0.0"
	}
	if err := r.Registry.Store(ctx, prompt); err != nil {
		logger.Error(err, "failed to store prompt in registry")
		cr.Status.Synced = false
		cr.Status.Message = err.Error()
		_ = r.Status().Update(ctx, cr)
		return ctrl.Result{}, err
	}
	if cr.Spec.Stage != "" && cr.Spec.Stage != "dev" {
		_ = r.Registry.Promote(ctx, prompt.ID, prompt.Version, registry.Stage(cr.Spec.Stage))
	}
	cr.Status.Synced = true
	cr.Status.LastSyncTime = time.Now().UTC().Format(time.RFC3339)
	cr.Status.Message = ""
	if err := r.Status().Update(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("synced prompt to registry", "id", prompt.ID, "version", prompt.Version)
	return ctrl.Result{}, nil
}

func crToPrompt(cr *v1.Prompt) *core.Prompt {
	p := &core.Prompt{
		ID:          cr.Spec.ID,
		Version:     cr.Spec.Version,
		Name:        cr.Spec.Name,
		Description: cr.Spec.Description,
		System:      cr.Spec.System,
		Template:    cr.Spec.Template,
		CreatedAt:   cr.CreationTimestamp.Time,
		UpdatedAt:   time.Now(),
	}
	if p.ID == "" {
		p.ID = cr.Name
	}
	for _, v := range cr.Spec.Variables {
		p.Variables = append(p.Variables, core.Variable{
			Name:        v.Name,
			Type:        core.VariableType(v.Type),
			Required:    v.Required,
			Default:     v.Default,
			Description: v.Description,
		})
	}
	if cr.Spec.Metadata != nil {
		p.Metadata = make(map[string]interface{})
		for k, val := range cr.Spec.Metadata {
			p.Metadata[k] = val
		}
	}
	return p
}

// SetupWithManager registers the reconciler with the manager.
func (r *PromptReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Prompt{}).
		Complete(r)
}

// NewScheme returns a scheme with loom types registered.
func NewScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add loom scheme: %w", err)
	}
	return scheme, nil
}
