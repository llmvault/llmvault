# Vercel DNS certificates

The public Gateway uses cert-manager with the Vercel DNS-01 webhook so it can
issue and renew wildcard certificates. The pinned webhook chart is rendered to
an ignored manifest because Kubernetes applies, rather than Helm, owns the
installation.

```bash
helm template cert-manager-webhook-vercel \
  /tmp/cert-manager-webhook-vercel-v1.0.0.tgz \
  --namespace cert-manager \
  --values kubernetes/bootstrap/cert-manager/vercel-webhook-values.yaml \
  > ansible/.secrets/k8s0/operators/cert-manager-webhook-vercel-v1.0.0.yaml

kubectl apply -f ansible/.secrets/k8s0/operators/cert-manager-webhook-vercel-v1.0.0.yaml
kubectl apply -f kubernetes/bootstrap/cert-manager/vercel-cluster-issuer.yaml
kubectl apply -k kubernetes/ingress/public
```

The `vercel-credentials` Secret belongs in `cert-manager` and is intentionally
not stored in Git. Its `token` key must contain a Vercel API token that can
manage the `usehivy.com` DNS zone.
