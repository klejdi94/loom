# Kubernetes operator

The prompt-operator syncs **Prompt** custom resources from the cluster into a loom registry (in-memory by default).

## CRD

Apply the CustomResourceDefinition:

```bash
kubectl apply -f deploy/prompt-crd.yaml
```

Group: `loom.klejdi94.github.com`, version: `v1`, resource: `prompts`.

## Prompt spec

- **id** (optional): Prompt ID; defaults to the resource `metadata.name`.
- **version**: Semantic version (e.g. `1.0.0`).
- **name**, **description**: Human-readable labels.
- **system**: System message template.
- **template**: User message template (Go `text/template`).
- **variables**: List of `{ name, type, required, description }`.
- **stage** (optional): `dev`, `staging`, or `production`; if set, the controller promotes that version to the given stage after storing.
- **metadata** (optional): String key-value map.

## Example

```yaml
apiVersion: loom.klejdi94.github.com/v1
kind: Prompt
metadata:
  name: my-prompt
  namespace: default
spec:
  id: my-prompt
  version: "1.0.0"
  name: Greeter
  system: "You are a helpful assistant."
  template: "Hello, {{.name}}!"
  variables:
    - name: name
      type: string
      required: true
  stage: production
```

## Running the operator

Build and run (e.g. locally with `KUBECONFIG` set):

```bash
go build -o prompt-operator ./cmd/prompt-operator
./prompt-operator
```

The operator uses an **in-memory registry** by default. To plug in a different registry (e.g. Redis or file), extend `cmd/prompt-operator/main.go` and pass the registry into `PromptReconciler`.

## Status

The controller updates `status.synced`, `status.lastSyncTime`, and `status.message` after each reconcile.
