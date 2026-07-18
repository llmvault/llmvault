# Public Gateway smoke stack

This Kustomize package is a pre-DNS bootstrap check. It creates a self-signed
certificate, a Cilium `Gateway`, HTTP-to-HTTPS redirect and an echo backend for
`gateway-check.usehivy.com`.

Apply and validate it with the commands in `../../bootstrap/README.md`. The
hostname does not need a DNS record because the checks use `curl --resolve`.

This stack may remain installed while platform manifests are prepared. Remove
it after a real public Gateway and trusted certificate have passed the same
checks:

```sh
kubectl delete -k kubernetes/smoke/gateway
```
