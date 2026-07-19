# Automated application deployments

Application image rollouts use one namespace-local ServiceAccount per
environment. Each account can only read and patch the `backend-api`,
`backend-worker`, and `web` Deployments in its own namespace. It cannot list
Deployments or access Pods, Secrets, ConfigMaps, data stores, or cluster-scoped
resources.

The Kubernetes API remains private. GitHub Actions opens an SSH tunnel through
either K3s server using an environment-specific Unix account whose authorized
key is restricted to forwarding `127.0.0.1:6443`. Kubernetes RBAC remains the
authorization boundary after the tunnel is established.

The `staging` and `production` GitHub Environments each contain:

- `KUBE_CONFIG_B64` secret: namespace-specific ServiceAccount kubeconfig;
- `K8S_TUNNEL_SSH_KEY_B64` secret: restricted tunnel private key;
- `K8S_TUNNEL_KNOWN_HOSTS_B64` secret: pinned K3s server SSH host keys;
- `K8S_TUNNEL_HOSTS` variable: ordered public K3s server addresses;
- `K8S_TUNNEL_USER` variable: environment-specific restricted SSH user.

Pushes to `main` deploy immutable backend and web image digests to staging after
both images are published. Published stable releases build versioned backend
and web images and deploy their immutable digests to production. Prereleases do
not deploy to production.
