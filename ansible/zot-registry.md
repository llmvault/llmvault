# Zot Registry Setup

This documents the manual Zot registry setup performed on the Hetzner VPS at
`157.180.117.84`.

## Host

- Hostname: `registry`
- Public IP: `157.180.117.84`
- Private IP: `10.80.0.3`
- OS: Ubuntu 26.04 LTS
- Volume: `/dev/disk/by-id/scsi-0HC_Volume_106075040`
- Volume mount: `/mnt/HC_Volume_106075040`
- Zot storage: `/mnt/HC_Volume_106075040/zot`
- Public registry name used by runners and MSB: `registry.usehivy.com:5000`
- Private registry proxy listen address: `10.80.0.3:5000` over HTTPS
- Zot upstream listen address: `127.0.0.1:5001` over HTTP

The registry is intentionally bound to the private interface only. UFW allows
SSH and allows registry port `5000/tcp` only from `10.80.0.0/16`.

## Install Zot

Zot `v2.1.17` minimal Linux amd64 was installed as a native systemd service:

```sh
ZOT_VERSION=v2.1.17
ZOT_SHA=523e5bf29a013db09115f780c3152af98fc5b65fc408a0d3e6c293643dc9bde7
ZOT_URL="https://github.com/project-zot/zot/releases/download/${ZOT_VERSION}/zot-linux-amd64-minimal"

install -d -m 0755 /etc/zot
install -d -m 0755 /usr/local/bin
install -d -m 0755 /mnt/HC_Volume_106075040/zot

useradd --system --home-dir /var/lib/zot --shell /usr/sbin/nologin zot
chown -R zot:zot /mnt/HC_Volume_106075040/zot

curl -fsSL "$ZOT_URL" -o /tmp/zot-linux-amd64-minimal
echo "$ZOT_SHA  /tmp/zot-linux-amd64-minimal" | sha256sum -c -
install -o root -g root -m 0755 /tmp/zot-linux-amd64-minimal /usr/local/bin/zot
rm -f /tmp/zot-linux-amd64-minimal
```

## TLS Proxy

Microsandbox pulls OCI images over HTTPS and does not trust a private/self-signed
CA. The registry therefore uses Caddy with DNS-01 ACME to serve a public trusted
certificate for `registry.usehivy.com:5000`, while runners map that hostname to
the private registry IP with `/etc/hosts`.

Deploy the TLS proxy with:

```sh
ansible-playbook -i inventory/hosts.yml playbooks/phase2c-registry-proxy.yml
ansible-playbook -i inventory/hosts.yml playbooks/phase1-prepare.yml
```

The registry proxy role installs Caddy with the Vercel DNS module, configures Zot
as a localhost-only HTTP upstream, and binds Caddy to `10.80.0.3:5000`.
The runner-base role maps `registry.usehivy.com` to `10.80.0.3` on runner hosts.

## Config

`/etc/zot/config.json`:

```json
{
  "distSpecVersion": "1.1.1",
  "storage": {
    "rootDirectory": "/mnt/HC_Volume_106075040/zot",
    "dedupe": true,
    "gc": true,
    "gcDelay": "1h",
    "gcInterval": "24h",
    "commit": true
  },
  "http": {
    "address": "127.0.0.1",
    "port": "5001"
  },
  "log": {
    "level": "info"
  }
}
```

## Systemd

`/etc/systemd/system/zot.service`:

```ini
[Unit]
Description=Zot OCI registry
Documentation=https://zotregistry.dev/
After=network-online.target mnt-HC_Volume_106075040.mount
Wants=network-online.target
Requires=mnt-HC_Volume_106075040.mount

[Service]
Type=simple
User=zot
Group=zot
ExecStart=/usr/local/bin/zot serve /etc/zot/config.json
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/mnt/HC_Volume_106075040/zot
CapabilityBoundingSet=
LockPersonality=true
MemoryDenyWriteExecute=true

[Install]
WantedBy=multi-user.target
```

Enable and start:

```sh
systemctl daemon-reload
systemctl enable --now zot
```

## Firewall

```sh
ufw allow OpenSSH
ufw allow from 10.80.0.0/16 to any port 5000 proto tcp
ufw --force enable
```

Expected rules:

```text
OpenSSH   ALLOW IN  Anywhere
5000/tcp  ALLOW IN  10.80.0.0/16
OpenSSH   ALLOW IN  Anywhere (v6)
```

Expected listener:

```text
10.80.0.3:5000   # Caddy TLS proxy
127.0.0.1:5001   # Zot HTTP upstream
```

There should be no public listener on `157.180.117.84:5000`.

## Validation

Local service checks on the registry host:

```sh
systemctl is-active zot
systemctl is-enabled zot
/usr/local/bin/zot --version
findmnt /mnt/HC_Volume_106075040 -o TARGET,SOURCE,FSTYPE,OPTIONS
df -hT / /mnt/HC_Volume_106075040
```

Private registry health check from a runner:

```sh
getent hosts registry.usehivy.com
curl -fsS --max-time 5 https://registry.usehivy.com:5000/v2/
```

Public check from outside the private network should fail or time out:

```sh
curl --connect-timeout 5 --max-time 5 -I https://157.180.117.84:5000/v2/
```

Private-network push/pull test used a temporary ORAS binary from runner
`10.80.1.3`:

```sh
work=/tmp/zot-smoke-test
rm -rf "$work"
mkdir -p "$work/bin" "$work/out"
cd "$work"

curl -fsSL https://github.com/oras-project/oras/releases/download/v1.3.2/oras_1.3.2_linux_amd64.tar.gz -o oras.tar.gz
tar -xzf oras.tar.gz -C bin oras

printf "hivy zot smoke %s\n" "$(date -u +%Y%m%dT%H%M%SZ)" > payload.txt
./bin/oras push registry.usehivy.com:5000/hivy/zot-smoke:private-net payload.txt:text/plain
./bin/oras pull -o out registry.usehivy.com:5000/hivy/zot-smoke:private-net
cmp payload.txt out/payload.txt

rm -rf "$work"
```

Runner `10.80.1.2` was also able to pull the same artifact:

```sh
./bin/oras pull -o out registry.usehivy.com:5000/hivy/zot-smoke:private-net
```

The smoke artifact created these files on the Hetzner Volume:

```text
/mnt/HC_Volume_106075040/zot/hivy/zot-smoke
```

## Operations

Useful commands:

```sh
journalctl -u zot -f
systemctl restart zot
du -h -d 3 /mnt/HC_Volume_106075040/zot | sort -h | tail
ufw status numbered
ss -tulpn | grep 5000
```
