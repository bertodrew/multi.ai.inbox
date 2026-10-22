# Deploy runbook for an operations agent (Hermes Agent or similar)

Audience: an AI agent with access to the Hetzner Cloud API (`hcloud` CLI or API token) and SSH, acting for the founder. Every step has a check. Steps marked **STOP** need an explicit "yes" from the founder before they run.

Target: the existing CX33 at `2.28.99.252` (staging, no customers), reinstalled with Ubuntu 24.04, running k3s + Flux GitOps from `bertodrew/multi.ai.inbox`, branch `master`, path `deploy/flux/clusters/staging`.

## 0. Inputs the agent must receive (never commit them, never print them in logs)

| Variable | What | Where the founder gets it |
|----------|------|---------------------------|
| `HCLOUD_TOKEN` | Hetzner Cloud API token, read/write, project of the CX33 | Hetzner console > project > Security > API tokens |
| `GITHUB_TOKEN` | Fine grained PAT on `bertodrew/multi.ai.inbox`: Contents read/write, Administration read/write | GitHub > Settings > Developer settings |
| `GHCR_TOKEN` | Classic PAT with `read:packages` | same |
| `BREVO_LOGIN`, `BREVO_KEY` | Brevo SMTP login and SMTP key | Brevo > SMTP & API > SMTP |
| `BACKUP_S3_ENDPOINT`, `BACKUP_S3_BUCKET`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` | Hetzner Object Storage (optional; backups fail until set) | Hetzner console > Object Storage |
| `ADMIN_IP` | Public IP allowed to SSH (the founder's, and the agent's if it runs elsewhere) | `curl ifconfig.me` on that machine |
| `SSH_KEY_NAME` | Name of the founder's SSH key already in the Hetzner project | `hcloud ssh-key list` |

## 0b. Before the merge: save the landing (read only on the server)

The CX33 also serves the current landing page next to mailcow. The rebuild wipes it, so copy it into the repository first; the new stack serves `landing/` from the Caddy image.

1. Find the landing root on the server, read only:
   `ssh root@2.28.99.252 'grep -rhoE "root[[:space:]]+[^;]+" /etc/nginx /opt/*/data/conf/nginx 2>/dev/null | sort -u; docker ps --format "{{.Names}} {{.Image}}"'`
   and look for the directory that holds the landing `index.html` (not `/web`, which is the mailcow admin UI).
2. Copy it into a checkout of the PR branch:
   `git clone -b claude/nice-franklin-5o6p0r https://github.com/bertodrew/multi.ai.inbox && rsync -av root@2.28.99.252:<landing root>/ multi.ai.inbox/landing/`
   Keep `landing/README.md`. Remove server side files that must not be published (`*.php` with credentials, `.env`, `config*.php`, keys). Only static files are served.
3. `grep -rniE "password|secret|api[_-]?key|token" landing/` must return nothing sensitive. If it does, stop and ask the founder.
4. Commit and push to the PR branch: `git add landing && git commit -m "feat: import current landing page" && git push`.
5. Tell the founder the PR is ready to merge.

Check: `landing/index.html` on the PR branch is the real landing, and the PR CI is green. Merging the PR does not touch the server; only step 2 (rebuild) does.

## 1. Preconditions

1. PR `bertodrew/multi.ai.inbox#1` is merged into `master` (the founder merges it).
2. The `release` workflow on `master` finished green, so the image exists:
   `gh run list --repo bertodrew/multi.ai.inbox --workflow release --limit 1` shows `completed success`,
   and `deploy/k8s/staging/kustomization.yaml` on `master` has `newTag:` set to a commit SHA (not `latest`).
3. Find the server: `hcloud server list -o columns=id,name,ipv4,status` and identify the one with IPv4 `2.28.99.252`. Store its name as `SERVER`.

Check: all three are true. If not, stop and report which one failed.

## 2. Snapshot, then rebuild (STOP)

1. Snapshot for rollback: `hcloud server create-image --type snapshot --description "pre-k3s mailcow staging" $SERVER`.
   Check: `hcloud image list --type snapshot` shows it as `available`.
2. Verify SSH access survives the rebuild: `hcloud ssh-key list -o columns=name` shows `$SSH_KEY_NAME`,
   and it is attached to the server: `hcloud server describe $SERVER -o json | jq -r '.ssh_keys[].name'`
   lists `$SSH_KEY_NAME`. If it is not attached, the rebuild has no key to inject and its response
   carries a root password: keep it and use it once to install the SSH key.
3. **STOP**: ask the founder "Rebuild $SERVER (2.28.99.252) with Ubuntu 24.04? This wipes mailcow staging. Snapshot <id> exists. SSH key $SSH_KEY_NAME attached: <yes/no>." Continue only on yes.
4. Rebuild, keeping the IP: `hcloud server rebuild $SERVER --image ubuntu-24.04`.
   If the command prints a root password, use it once to install the SSH key; otherwise the attached SSH keys are re-injected.
   If SSH then refuses the key and no root password was printed, do not rebuild again: run
   `hcloud server reset-password $SERVER` (requires the server running), log in once with that password
   through the Hetzner web console, add the key to `/root/.ssh/authorized_keys`, then continue.
5. Wait for SSH: `until ssh -o StrictHostKeyChecking=accept-new root@2.28.99.252 true; do sleep 5; done`.
   The host key changed with the rebuild: remove the old one first with `ssh-keygen -R 2.28.99.252`.

Check: `ssh root@2.28.99.252 'lsb_release -rs'` prints `24.04`.

## 3. Firewall and reverse DNS

```bash
hcloud firewall create --name mai-staging
hcloud firewall add-rule mai-staging --direction in --protocol tcp --port 22  --source-ips "$ADMIN_IP/32" --description ssh
hcloud firewall add-rule mai-staging --direction in --protocol tcp --port 25  --source-ips 0.0.0.0/0 --source-ips ::/0 --description smtp
hcloud firewall add-rule mai-staging --direction in --protocol tcp --port 80  --source-ips 0.0.0.0/0 --source-ips ::/0 --description acme
hcloud firewall add-rule mai-staging --direction in --protocol tcp --port 443 --source-ips 0.0.0.0/0 --source-ips ::/0 --description https
hcloud firewall add-rule mai-staging --direction in --protocol icmp --source-ips 0.0.0.0/0 --source-ips ::/0 --description ping
hcloud firewall apply-to-resource mai-staging --type server --server $SERVER
hcloud server set-rdns $SERVER --ip 2.28.99.252 --hostname mx.2-28-99-252.sslip.io
```

If the agent runs from a host other than `ADMIN_IP`, add that host's IP to the SSH rule too, or the next step locks the agent out.

Check: `hcloud firewall describe mai-staging` lists the five rules and the server; `dig +short -x 2.28.99.252` returns `mx.2-28-99-252.sslip.io.`

## 4. Bootstrap k3s, secrets and Flux

Run non interactively over SSH. Secrets travel in the SSH session environment only:

```bash
scp deploy/hetzner/bootstrap.sh root@2.28.99.252:/root/bootstrap.sh
ssh root@2.28.99.252 \
  "GITHUB_TOKEN='$GITHUB_TOKEN' GHCR_TOKEN='$GHCR_TOKEN' BREVO_LOGIN='$BREVO_LOGIN' BREVO_KEY='$BREVO_KEY' \
   BACKUP_S3_ENDPOINT='${BACKUP_S3_ENDPOINT:-}' BACKUP_S3_BUCKET='${BACKUP_S3_BUCKET:-}' \
   BACKUP_S3_ACCESS_KEY='${BACKUP_S3_ACCESS_KEY:-}' BACKUP_S3_SECRET_KEY='${BACKUP_S3_SECRET_KEY:-}' \
   bash /root/bootstrap.sh < /dev/null" 2>&1 | tee bootstrap.log
```

The script installs k3s and Flux, creates the secrets, runs `flux bootstrap github` (this commits `deploy/flux/clusters/staging/flux-system` to `master`), waits for the rollout, runs `mai admin seed` and `mai admin check-port25`.

Database passwords need no input: the script generates them into secret `mai-secrets` (`POSTGRES_PASSWORD`, `MAI_OWNER_PASSWORD` for migrations only, `MAI_APP_PASSWORD` for tenant requests, `MAI_SYSTEM_PASSWORD` for workers). Rerunning it on a cluster created before the role split adds the two missing passwords, creates the roles and hands table ownership from `mai_app` to `mai_owner`. Migrations run in the `migrate` init container of the `mai` pod (`kubectl -n mai logs deploy/mai -c migrate`).

**Handle the seed output as a secret**: it contains the two tester API keys, printed once. Give them to the founder privately, never in a public channel, issue or commit.

Checks:
* `ssh root@2.28.99.252 'kubectl get nodes'` shows one `Ready` node.
* `ssh root@2.28.99.252 'flux get kustomizations -A'` shows `flux-system` and `mai` with `Ready True`.
* `ssh root@2.28.99.252 'kubectl -n mai get pods'`: `postgres-0`, `mai-*`, `rspamd-*`, `caddy-*` (landing + TLS) all `Running`.
* `curl -fsS https://api.2-28-99-252.sslip.io/healthz` returns HTTP 204 (the certificate can take up to a minute).
* `curl -fsS https://2-28-99-252.sslip.io/` and `curl -fsS http://2.28.99.252/` return the landing page.

## 5. Smoke test

With `KEY` = the user.tester API key from the seed output:

```bash
API=https://api.2-28-99-252.sslip.io
curl -fsS $API/v0/auth/me -H "Authorization: Bearer $KEY"
curl -fsS -X POST $API/v0/inboxes/user.tester@agents.localhost/messages/send -H "Authorization: Bearer $KEY" \
  -d '{"to":["admin.tester@agents.localhost"],"subject":"smoke","text":"hello from the runbook"}'
sleep 5
curl -fsS "$API/v0/inboxes/admin.tester@agents.localhost/messages" -H "Authorization: Bearer $KEY"
```

Check: the last call lists the `smoke` message with labels including `received`.

MCP check: `curl -fsS -X POST $API/mcp -H "X-Api-Key: $KEY" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'` lists `send_message`.

## 6. Outbound mail

Read the `check-port25` line from `bootstrap.log`:
* `OPEN`: direct delivery works. Optionally remove the relay: `kubectl -n mai patch secret mai-secrets --type json -p '[{"op":"remove","path":"/data/MAI_SMTP_RELAY"}]'` then `kubectl -n mai rollout restart deploy/mai`.
* `BLOCKED`: keep the Brevo relay. Tell the founder they can request unblocking in the Hetzner console (Support > Limits) if they want direct delivery later.

Note: until the brand domain exists, inboxes live on `agents.localhost`; mail between agents works, mail to and from the internet does not. That is expected.

## 7. Report back to the founder

One message with: snapshot id, firewall name, PTR result, Flux status, pod status, healthz result, smoke test result, port 25 result, where the tester keys were delivered, anything that failed and what was done about it.

## Rollback

* Platform problem after a deploy: `git revert` the bad commit on `master`; Flux rolls back within 5 minutes. Or pin the previous SHA in `deploy/k8s/staging/kustomization.yaml`.
* Server problem: `hcloud server rebuild $SERVER --image <snapshot id>` restores the pre-k3s state (mailcow staging).

## Day two operations

| Task | Command (on the server) |
|------|-------------------------|
| Status | `flux get kustomizations -A && kubectl -n mai get pods` |
| Logs | `kubectl -n mai logs deploy/mai --since=1h` |
| Force sync | `flux reconcile kustomization mai --with-source` |
| New org / key | `kubectl -n mai exec deploy/mai -- mai admin org-create -name Acme` then `... key-create -org <id>` |
| Change plan | `kubectl -n mai exec deploy/mai -- mai admin org-plan -org <id> -plan team` |
| Run a backup now | `kubectl -n mai create job --from=cronjob/backup backup-manual-$(date +%s)` |
| Rotate Brevo key | `kubectl -n mai create secret generic mai-secrets ... --dry-run=client -o yaml \| kubectl apply -f -` keeping the four database passwords, then `kubectl -n mai rollout restart deploy/mai` |

Rules for the agent: never run `kubectl delete namespace mai`, never delete PVCs, never print secrets, never force push to `master`. Ask the founder before any destructive Hetzner action (rebuild, delete, firewall removal).
