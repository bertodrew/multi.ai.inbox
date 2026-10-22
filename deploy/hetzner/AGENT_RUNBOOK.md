# Deploy runbook for an operations agent (Hermes Agent or similar)

Audience: an AI agent with access to the Hetzner Cloud API (`hcloud` CLI or API token) and SSH, acting for the founder. Every step has a check. Steps marked **STOP** need an explicit "yes" from the founder before they run.

Target: the existing CX33 at `2.28.99.252` (staging, no customers), reinstalled with Ubuntu 24.04, running k3s + Flux GitOps from `bertodrew/multi.ai.inbox`, branch `master`, path `deploy/flux/clusters/staging`.

## 0. Inputs the agent must receive (never commit them, never print them in logs)

| Variable | What | Where the founder gets it |
|----------|------|---------------------------|
| `HCLOUD_TOKEN` | Hetzner Cloud API token, read/write, **only** for the project that holds the staging CX33 (section 0a), deleted after the deploy | Hetzner console > project > Security > API tokens |
| GitHub access for Flux | **No token by default**: `bootstrap.sh` creates an SSH deploy key and the founder adds it in the repository settings (section 4). Legacy alternative: `GITHUB_TOKEN`, see section 0a | GitHub > repository > Settings > Deploy keys |
| `GHCR_TOKEN` | Classic PAT with `read:packages` and nothing else, separate from any other token, with an expiry | GitHub > Settings > Developer settings |
| `BREVO_LOGIN`, `BREVO_KEY` | Brevo SMTP login and SMTP key | Brevo > SMTP & API > SMTP |
| `BACKUP_S3_ENDPOINT`, `BACKUP_S3_BUCKET`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` | Hetzner Object Storage: the Object Lock bucket and its write only key (section 4b; optional, backups fail until set) | Hetzner console > Object Storage |
| `BACKUP_CRYPT_PASSWORD`, `BACKUP_CRYPT_SALT` | Backup encryption keys. **Leave unset on the first deploy**: `bootstrap.sh` generates them and prints them once for the founder to store offline. Pass the stored values on any later rebuild or restore | the founder's offline copy |
| `ADMIN_IP` | Public IP allowed to SSH (the founder's, and the agent's if it runs elsewhere) | `curl ifconfig.me` on that machine |
| `SSH_KEY_NAME` | Name of the founder's SSH key already in the Hetzner project | `hcloud ssh-key list` |

## 0a. Least privilege tokens (read before asking the founder for any token)

* **GitHub, Flux.** `flux bootstrap github` needs a fine grained PAT on this one repository with Contents read/write, Metadata read and Administration read/write (Administration only to create the deploy key; with `--token-auth` it would need Administration read only, but then the PAT itself is stored in the cluster). Per the [Flux docs](https://fluxcd.io/flux/installation/bootstrap/github/) the deploy key created that way **stops working when the PAT is revoked or expires**, so the PAT cannot simply be revoked after bootstrap. Therefore `bootstrap.sh` defaults to `flux bootstrap git` with an SSH deploy key that the founder adds by hand: no GitHub token reaches the agent or the server at all. The key needs write access once (bootstrap pushes `flux-system` manifests); afterwards the founder deletes it and adds the same public key again without write access. If the legacy PAT path is used anyway, give it a 7 day expiry and plan the switch to a deploy key before it expires.
* **GHCR.** A separate classic PAT with `read:packages` only (GHCR pulls do not take fine grained tokens). It stays in the cluster as secret `ghcr-pull`: set an expiry (90 days) and rotate it with the command in "Day two operations".
* **Hetzner Cloud.** API tokens are per project and a read/write token can delete every server, volume, snapshot and Primary IP in that project. Keep staging in a **dedicated Hetzner project** that holds nothing else. If the CX33 currently shares a project with other servers, tell the founder before step 2: the options are a new CX33 in a new project (new IP, so new sslip.io names in `deploy/k8s/staging/kustomization.yaml`) or accepting the shared project for this one deploy. Create the read/write token right before step 1b and **delete it after section 7** (Hetzner console > project > Security > API tokens); day two work uses a read only token. Never use a token from the project that holds the backup bucket (section 4b).
* **Object Storage.** The backup bucket lives in a separate Hetzner project the agent has no token for; the cluster gets a key that can add objects but not delete them (section 4b).

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

## 1b. Protect the IP and turn on server backups (before the rebuild)

The IPv4 `2.28.99.252` is a Primary IP. With auto delete on (the default for IPs created with the server) it is released when the server is deleted, and every sslip.io name and the PTR are lost with it.

```bash
IP_ID=$(hcloud primary-ip list -o json | jq -r '.[] | select(.ip == "2.28.99.252") | .id')
hcloud primary-ip update "$IP_ID" --auto-delete=false
hcloud primary-ip enable-protection "$IP_ID" delete
hcloud server enable-backup $SERVER
```

`enable-backup` turns on Hetzner's daily disk backups (7 slots, 20 % of the server price, about €1.70/month for the CX33). They are a second layer next to the encrypted Object Storage backups, not a replacement: they live in the same project and die with it.

Do **not** enable server rebuild protection yet: it would block step 2. Server delete and rebuild protection are enabled together right after the rebuild (step 2.6).

Check: `hcloud primary-ip describe "$IP_ID" -o json | jq '{auto_delete, protection}'` shows `"auto_delete": false` and `"delete": true`; `hcloud server describe $SERVER -o json | jq .backup_window` is not `null`.

## 2. Snapshot, then rebuild (STOP)

1. Snapshot for rollback: `hcloud server create-image --type snapshot --description "pre-k3s mailcow staging" $SERVER`.
   Check: `hcloud image list --type snapshot` shows it as `available`.
2. Verify SSH access survives the rebuild: `hcloud ssh-key list -o columns=name` shows `$SSH_KEY_NAME`,
   and it is attached to the server: `hcloud server describe $SERVER -o json | jq -r '.ssh_keys[].name'`
   lists `$SSH_KEY_NAME`. If it is not attached, the rebuild has no key to inject and its response
   carries a root password: keep it and use it once to install the SSH key.
3. **STOP**: ask the founder "Rebuild $SERVER (2.28.99.252) with Ubuntu 24.04? This wipes mailcow staging. Snapshot <id> exists. SSH key $SSH_KEY_NAME attached: <yes/no>." Continue only on yes.
4. Rebuild, keeping the IP: `hcloud server rebuild $SERVER --image ubuntu-24.04`.
   Rebuild protection must be off for this (it is, unless someone enabled it; check with
   `hcloud server describe $SERVER -o json | jq .protection`, and ask the founder before running
   `hcloud server disable-protection $SERVER delete rebuild`).
   If the command prints a root password, use it once to install the SSH key; otherwise the attached SSH keys are re-injected.
   If SSH then refuses the key and no root password was printed, do not rebuild again: run
   `hcloud server reset-password $SERVER` (requires the server running), log in once with that password
   through the Hetzner web console, add the key to `/root/.ssh/authorized_keys`, then continue.
5. Wait for SSH: `until ssh -o StrictHostKeyChecking=accept-new root@2.28.99.252 true; do sleep 5; done`.
   The host key changed with the rebuild: remove the old one first with `ssh-keygen -R 2.28.99.252`.
6. Protect the rebuilt server against deletion and a second rebuild:
   `hcloud server enable-protection $SERVER delete rebuild`
   (Hetzner keeps the two flags together; any later rebuild or delete first needs
   `hcloud server disable-protection $SERVER delete rebuild`, which is a **STOP** step.)

Check: `ssh root@2.28.99.252 'lsb_release -rs'` prints `24.04`; `hcloud server describe $SERVER -o json | jq .protection` shows `"delete": true, "rebuild": true`.

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
  "GHCR_TOKEN='$GHCR_TOKEN' BREVO_LOGIN='$BREVO_LOGIN' BREVO_KEY='$BREVO_KEY' \
   BACKUP_S3_ENDPOINT='${BACKUP_S3_ENDPOINT:-}' BACKUP_S3_BUCKET='${BACKUP_S3_BUCKET:-}' \
   BACKUP_S3_ACCESS_KEY='${BACKUP_S3_ACCESS_KEY:-}' BACKUP_S3_SECRET_KEY='${BACKUP_S3_SECRET_KEY:-}' \
   BACKUP_CRYPT_PASSWORD='${BACKUP_CRYPT_PASSWORD:-}' BACKUP_CRYPT_SALT='${BACKUP_CRYPT_SALT:-}' \
   bash /root/bootstrap.sh < /dev/null" 2>&1 | tee bootstrap.log
```

**First run stops on purpose (exit status 3)** and prints an SSH public key. Send it to the founder: "Add this as a deploy key on `bertodrew/multi.ai.inbox` (Settings > Deploy keys), title `flux-staging`, Allow write access ticked." When the founder confirms, run the same command again; it continues where it stopped. After the script finishes, ask the founder to delete that deploy key and add the same public key again **without** write access (Flux only reads; the key also lives in the cluster secret `flux-system/flux-system`).

The script installs k3s and Flux, creates the secrets, runs `flux bootstrap git` with the deploy key (this commits `deploy/flux/clusters/staging/flux-system` to `master`), waits for the rollout, creates the read only Postgres role `mai_backup` for the backup job, runs `mai admin seed` and `mai admin check-port25`.

**Handle two outputs as secrets**, never in a public channel, issue, commit or long lived log:
* the seed output: the two tester API keys, printed once. Give them to the founder privately.
* the `BACKUP ENCRYPTION KEYS` block (only when they were generated, i.e. on the first deploy). Without these two values **no backup can ever be restored**. Give the block to the founder privately, ask them to confirm they stored it offline (password manager plus paper), then delete `bootstrap.log` and any other copy on the agent host. Until the founder confirms, report the deploy as not finished.

Checks:
* `ssh root@2.28.99.252 'kubectl get nodes'` shows one `Ready` node.
* `ssh root@2.28.99.252 'flux get kustomizations -A'` shows `flux-system` and `mai` with `Ready True`.
* `ssh root@2.28.99.252 'kubectl -n mai get pods'`: `postgres-0`, `mai-*`, `rspamd-*`, `caddy-*` (landing + TLS) all `Running`.
* `curl -fsS https://api.2-28-99-252.sslip.io/healthz` returns HTTP 204 (the certificate can take up to a minute).
* `curl -fsS https://2-28-99-252.sslip.io/` and `curl -fsS http://2.28.99.252/` return the landing page.

## 4b. Backups: Object Lock bucket, write only key, first run

The founder does steps 1 to 3 (the agent has no token for the vault project). Full background in `docs/agentmail-re/14-disaster-recovery-backup.md` section 4.1.

1. **Vault project and bucket.** New Hetzner project `mai-backup-vault`, preferably in another location than the server (server in fsn1 or nbg1: bucket in hel1). Create the bucket **with Object Lock enabled at creation** (it cannot be added later), then set the default retention, for example with the AWS CLI configured for `https://hel1.your-objectstorage.com` and a key of the vault project:
   ```bash
   aws s3api create-bucket --bucket mai-staging-backup --region hel1 --object-lock-enabled-for-bucket
   aws s3api put-object-lock-configuration --bucket mai-staging-backup --object-lock-configuration \
     '{ "ObjectLockEnabled": "Enabled", "Rule": { "DefaultRetention": { "Mode": "GOVERNANCE", "Days": 30 }}}'
   ```
   `GOVERNANCE` can still be lifted by a key with the bypass permission (useful while staging is young); `COMPLIANCE` cannot be shortened by anyone, Hetzner included: switch to it once customers exist.
2. **Lifecycle** (the backup job never deletes, so expiry is the bucket's job): expire dumps after 45 days (longer than the 30 day lock), keep blobs.
   ```bash
   aws s3api put-bucket-lifecycle-configuration --bucket mai-staging-backup --lifecycle-configuration '{"Rules": [
     {"ID": "dumps", "Status": "Enabled", "Prefix": "crypt/dumps/", "Expiration": {"Days": 45},
      "NoncurrentVersionExpiration": {"NoncurrentDays": 1}},
     {"ID": "uploads", "Status": "Enabled", "Prefix": "", "AbortIncompleteMultipartUpload": {"DaysAfterInitiation": 7}}]}'
   ```
3. **Write only key for the cluster.** Create an S3 key in the **staging** project (not the vault). Keys only reach buckets of their own project, so grant this one key access with a bucket policy on the vault bucket, without any delete, retention or policy action (Hetzner user id format `p<project_id>:<access_key>`):
   ```json
   {"Version": "2012-10-17", "Statement": [{
     "Effect": "Allow",
     "Principal": {"AWS": "arn:aws:iam:::user/p<staging_project_id>:<access_key>"},
     "Action": ["s3:ListBucket", "s3:GetBucketLocation", "s3:GetObject", "s3:GetObjectVersion", "s3:PutObject",
                "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts", "s3:ListBucketMultipartUploads"],
     "Resource": ["arn:aws:s3:::mai-staging-backup", "arn:aws:s3:::mai-staging-backup/*"]}]}
   ```
   Apply it with `aws s3api put-bucket-policy --bucket mai-staging-backup --policy file://policy.json` (vault key). Read access is granted on purpose: the restore drill needs it, and the data is encrypted with keys the cluster already holds. Even if this key leaks, nothing can be deleted or overwritten in place (versioning plus Object Lock keep the old version).
4. **Agent: put the key into the cluster** if it was not passed to `bootstrap.sh`: rerun `bootstrap.sh` with the four `BACKUP_S3_*` variables (the crypt keys and everything else are kept).
5. **Agent: first backup and drill.**
   ```bash
   ssh root@2.28.99.252 'kubectl -n mai create job --from=cronjob/backup backup-first-$(date +%s)'
   # wait until it completes, then
   ssh root@2.28.99.252 'kubectl -n mai create job --from=cronjob/restore-drill restore-drill-first-$(date +%s)'
   ssh root@2.28.99.252 'kubectl -n mai logs job/<drill job name> --all-containers'
   ```
   Check: the backup log ends with `uploaded:` and the drill log prints `row counts (table|backup|restored)` with no `differs`, then `restore drill passed`. The founder then checks that the cluster's key really cannot delete: `aws s3api delete-object --bucket mai-staging-backup --key <any key from the listing>` with that key must fail with `AccessDenied`. If the first upload fails with an error about a missing `Content-MD5` header (some S3 servers require it for Object Lock buckets), report it to the founder instead of loosening the bucket.

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

One message with: snapshot id, Primary IP and server protection status, server backups on, firewall name, PTR result, Flux status, pod status, healthz result, smoke test result, port 25 result, first backup and restore drill result, where the tester keys and the backup encryption keys were delivered (and the founder's confirmation that the encryption keys are stored offline), anything that failed and what was done about it.

Then close the privileges: ask the founder to delete the read/write `HCLOUD_TOKEN` (or delete it yourself if the founder allows), to switch the deploy key to read only (section 4) and, if the legacy PAT path was used, to note its expiry date.

## Rollback

* Platform problem after a deploy: `git revert` the bad commit on `master`; Flux rolls back within 5 minutes. Or pin the previous SHA in `deploy/k8s/staging/kustomization.yaml`.
* Server problem (**STOP**, founder's yes first): `hcloud server disable-protection $SERVER delete rebuild`, then `hcloud server rebuild $SERVER --image <snapshot id>` restores the pre-k3s state (mailcow staging), then `hcloud server enable-protection $SERVER delete rebuild` again.
* Data problem: restore from the encrypted backups with `deploy/dr/restore.sh` (needs the founder's offline crypt keys), see `docs/agentmail-re/14-disaster-recovery-backup.md` section 5.3.

## Day two operations

| Task | Command (on the server) |
|------|-------------------------|
| Status | `flux get kustomizations -A && kubectl -n mai get pods` |
| Logs | `kubectl -n mai logs deploy/mai --since=1h` |
| Force sync | `flux reconcile kustomization mai --with-source` |
| New org / key | `kubectl -n mai exec deploy/mai -- mai admin org-create -name Acme` then `... key-create -org <id>` |
| Change plan | `kubectl -n mai exec deploy/mai -- mai admin org-plan -org <id> -plan team` |
| Run a backup now | `kubectl -n mai create job --from=cronjob/backup backup-manual-$(date +%s)` |
| Run the restore drill now | `kubectl -n mai create job --from=cronjob/restore-drill restore-drill-manual-$(date +%s)`, then `kubectl -n mai logs job/<name> --all-containers` |
| Backup and drill status | `kubectl -n mai get cronjobs,jobs` (`LAST SCHEDULE` per CronJob; a failed drill job stays listed for 7 days) |
| Rotate GHCR token | `kubectl -n mai create secret docker-registry ghcr-pull --docker-server=ghcr.io --docker-username=bertodrew --docker-password=<new> --dry-run=client -o yaml \| kubectl apply -f -`, then revoke the old PAT |
| Rotate Brevo key | `kubectl -n mai create secret generic mai-secrets ... --dry-run=client -o yaml \| kubectl apply -f -` keeping the two database passwords, then `kubectl -n mai rollout restart deploy/mai` |

Rules for the agent: never run `kubectl delete namespace mai`, never delete PVCs, never print secrets, never force push to `master`. Ask the founder before any destructive Hetzner action (rebuild, delete, firewall removal, disabling any protection). Never change or delete the `BACKUP_CRYPT_*` keys in secret `mai-backup`: new keys would make every earlier backup unreadable by the drill and by `restore.sh`.
