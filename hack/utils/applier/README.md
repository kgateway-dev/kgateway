# How to use this

Within this repo, you do not need a local `hack/utils/applier/go.mod`.
Go will use the repo root `go.mod`, even if you run from `hack/utils/applier`.

```bash
go run ./hack/utils/applier apply -f yamls.yaml --iterations 3000
```

This applies the template in `yamls.yaml` 3000 times.
The template has an `.Index` variable you can use.

For example, for a pod, you would have:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app-{{.Index}}
  namespace: kgateway-system
  labels:
    app: app-{{.Index}}
    test: test1
spec:
  containers:
  - name: app
    image: registry.k8s.io/pause:3.6
    resources:
      limits:
        memory: "1Mi"
        cpu: "1m"
    ports:
      - containerPort: 8080
```

This creates pods named `app-0`, `app-1`, and so on.

You can also:
- use `--dry-run` to just print yamls.
- adjust `--qps` and `--burst` as well.
- If you broke the cluster, you can use `--start` to continue from where you left off.

By default, objects will not be overridden. This is because
that a common failure mode here is that the cluster stops responding. When the cluster recovers you don't need to re-create the same object, so a simple create is enough. To change this behavior use `--force` to first delete objects and then re-create them.

To clean up (just delete the objects), pass the `--delete` flag.

If you are still not hitting the QPS set, try adding the `--async` flag to have requests going in parallel (you can also adjust `--workers`).

## Caveat

The template is parsed on the field level. This means that the input files must be valid yaml files. This is because the yaml is parsed **before** the template is evaluated.
Only after the yaml is parsed, we go over all the fields and evaluate the templates on each field.

This technical limitation comes from trying to provide an experience similar to `kubectl apply`.
