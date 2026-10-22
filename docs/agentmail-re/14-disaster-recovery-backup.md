# 14 · Disaster recovery, backup and resilience

Status: T0 backup and restore drill **implemented in git** (2026-10-22), waiting for the founder's bucket setup and the first run on the server; the rest is a proposal · Date: 2026-10-22 · Owner hat: CTO (network and Kubernetes) · Deciders: founder

Scope: the staging platform on one Hetzner CX33 (`2.28.99.252`, Germany) running single server k3s, Flux GitOps from `bertodrew/multi.ai.inbox`, Postgres 16 StatefulSet, the `mai` pod (api, mx, sender, dispatcher, enrich, scheduler), rspamd and Caddy. GitOps manifests: [`deploy/k8s/base/backup.yaml`](../../deploy/k8s/base/backup.yaml) and [`deploy/k8s/base/restore-drill.yaml`](../../deploy/k8s/base/restore-drill.yaml). Operator tools and the remaining proposal: [`deploy/dr/`](../../deploy/dr/README.md).

Tags as in the rest of this pack: **[observed]** read in this repo or a cited page, **[estimate]** our calculation, **[decision]** our recommendation. Prices exclude VAT.

**Implementation status** (2026-10-22):

| Item | Status |
|------|--------|
| Backup every 6 h: validated `pg_dump`, exact row counts from the same snapshot, blobs, `caddy-data` | done in `deploy/k8s/base/backup.yaml` |
| Client side encryption (rclone `crypt`, keys `BACKUP_CRYPT_PASSWORD` and `BACKUP_CRYPT_SALT` in secret `mai-backup`, generated once by `bootstrap.sh`, founder keeps them offline) | done |
| Add only uploads: `rclone copy` (never `sync` or `delete`), `--immutable` blobs, dated prefixes; the job needs no delete right | done |
| Read only Postgres role `mai_backup` for `pg_dump` (BYPASSRLS, `pg_read_all_data`, read only by default), created by `bootstrap.sh`; falls back to `postgres` when absent | done |
| Weekly restore drill into a scratch Postgres, row counts of every table compared with the backup, RLS and blob checks, dead man's switch | done in `deploy/k8s/base/restore-drill.yaml` |
| Object Lock bucket (governance 30 days) in a separate Hetzner project, write only key by bucket policy, lifecycle expiry | documented (AGENT_RUNBOOK section 4b), **founder action** |
| Primary IP auto delete off and delete protection, Hetzner server backups, server delete and rebuild protection | documented (AGENT_RUNBOOK sections 1b and 2), agent action at the next deploy |
| Ops agent tokens: no GitHub PAT (deploy key), dedicated Hetzner project, token deleted after the deploy | documented (AGENT_RUNBOOK section 0a) |
| `deploy/dr/restore.sh` updated for the encrypted layout | done, never run end to end |
| Default deny NetworkPolicy | proposal (`deploy/dr/networkpolicy-default-deny.yaml`) |
| Validation | `kustomize build deploy/k8s/staging \| kubeconform -strict` passes; the dump, upload, fetch, scratch restore, row count comparison and blob checks were run locally (Postgres 16, rclone 1.75.2 with a local remote under `crypt`); nothing has run on the real server or against Hetzner Object Storage yet |

The rest of this document keeps the original analysis; "today" in section 1 describes the state before this change.

## 1. Executive summary

**Today, honestly** [observed in `deploy/k8s/base`]:

| Item | Today |
|------|-------|
| RPO, database | up to 24 h (nightly `pg_dump -Fc` at 02:17 UTC). Plus: any mail accepted with `250` after the last dump is lost for good on a disk or node loss, because the sender no longer retries |
| RPO, blobs (raw `.eml`, attachments) | up to 24 h. Worse: `rclone sync` mirrors deletions and corruption into the backup the next night |
| RTO | several hours: nobody is alerted (no monitoring), then a manual rebuild (about 1 h of work [estimate]) that has never been rehearsed |
| Backup verification | none. A failing CronJob is only visible with `kubectl get jobs` |
| Backup protection | one copy, one provider, unencrypted (dumps contain DKIM private keys, webhook secrets, message bodies), credentials in the cluster can delete it |
| Not backed up | `caddy-data` (certificates, re-issuable), k3s state (reproducible from git, except the three secrets, which are re-creatable from provider consoles) |

**Target tiers** [decision]:

| Tier | When | RPO DB | RPO blobs | RTO | Inbound mail loss |
|------|------|--------|-----------|-----|-------------------|
| T0 staging | now, no customers | 6 h | 6 h | 4 h | accepted mail since last backup |
| T1 first customers (free, design partners) | brand domain bought | 5 min (WAL archiving) | 0 to 5 min (blobs on Object Storage) | 2 h | at most 5 min of accepted mail |
| T2 paid customers | first invoice | 5 min, PITR 14 days | 0 (S3 primary, versioned) | 30 min for a node, 2 h for a region | at most 5 min, backup MX later |

**Recommended path** [decision]:

* **Staging now (this week, about €2/month extra):** fix the blob `sync` to `copy`, back up every 6 h, dead man's switch on backup and on a weekly restore drill, default deny NetworkPolicy, encrypt backups client side (done with rclone `crypt`, section 4.1), new bucket with Object Lock in another location, Hetzner automatic backups for the disk, delete protection on server and Primary IP. Run `deploy/dr/restore.sh` once on a throwaway server. Status: see the table at the top; open are the bucket setup, the protections on the server, the default deny policy and the `restore.sh` dry run.
* **First customers:** move blobs to Object Storage (the planned S3 store in `internal/blob`), Postgres on CloudNativePG with Barman Cloud WAL archiving (RPO 5 min, PITR), Storage Box as second target, Floating IP for MX and API, status page.
* **Paid customers:** 3 node k3s with embedded etcd, CNPG primary plus replica on Hetzner volumes, stateless `mai` with PodDisruptionBudgets, etcd snapshots to S3, quarterly game day, optional backup MX.

## 2. Asset and data inventory

| Asset | Where today | Criticality | Recreatable? | Backed up today | Target protection |
|-------|-------------|-------------|--------------|-----------------|-------------------|
| Postgres `mai`: organizations, inboxes, domains, threads, messages (bodies and metadata), attachments rows | StatefulSet `postgres`, PVC `data-postgres-0` 20Gi local-path | **critical** | no | nightly dump | WAL archive + base backups, encrypted, immutable |
| Outbox state: `events`, `webhook_deliveries`, queued outbound `messages` | same | high | partly: replay produces duplicates | nightly | same; consumers dedupe on `event_id` |
| `audit_log` | same | high (security evidence) | no | nightly | same, retention ≥ 1 year later |
| API key hashes (`api_keys.hash`), webhook secrets | same | critical | keys can be reissued but break every customer | nightly | same; hashes are not secrets but secrets are |
| DKIM private keys (`domains.dkim_private_key`) | same, plaintext column | critical | regenerable, but customers must republish DNS | nightly, unencrypted | encrypt backups; later encrypt the column (KMS or age envelope) |
| Blob store: `raw/<org>/<yyyy>/<mm>/<inbox>-<msgid>.eml`, `att/<org>/<msg>/<n>` | PVC `mai-blobs` 50Gi local-path, `internal/blob.FS` | **critical** | no | nightly `rclone sync` | S3 primary (versioning + lock) |
| Secrets `mai-secrets`, `mai-backup`, `ghcr-pull` | cluster only, k3s `--secrets-encryption` | medium, except the backup crypt keys: **critical** | yes: DB passwords are regenerated by `bootstrap.sh` (roles are not in `pg_dump`), Brevo, S3 and GHCR tokens are reissued in their consoles. **Not** `BACKUP_CRYPT_PASSWORD` and `BACKUP_CRYPT_SALT`: lost keys mean every backup is lost | crypt keys: founder's offline copies | DR kit in the password manager: where to reissue each one, plus the two crypt keys (and on paper) |
| TLS certificates (API, site, MX STARTTLS) | PVC `caddy-data` 1Gi | low | yes, ACME | no | copy in backup to avoid rate limits |
| Cluster state (manifests, Flux) | GitHub `master` | high | yes from git | GitHub only | nightly `git clone --mirror` to Storage Box |
| Container images | GHCR (private), Docker Hub (postgres, rspamd, rclone) | medium | yes, CI rebuilds from git | GHCR only | pin by digest, keep last 10 SHAs |
| DNS | not owned (sslip.io names derived from the IP) | high later | n/a | n/a | registrar lock, 2FA, DNS export in git |
| Hetzner account, API tokens, 2FA recovery codes | founder | **critical** (single point of failure) | no | no | hardware key, recovery codes offline, separate backup project |
| GitHub account, PATs | founder, ops agent | critical | partly | no | same |

## 3. Failure scenarios

RPO/RTO "today" are for the current setup; "prevention" lists what this document proposes.

| # | Scenario | Detection | Impact | RPO / RTO today | Runbook | Prevention |
|---|----------|-----------|--------|-----------------|---------|------------|
| 1 | `mai` pod crash or OOM | liveness probe; restarts in `kubectl get pods` | API, SMTP down for seconds; senders retry on connection failure | 0 / 1 min | none needed; if crash loops: `kubectl -n mai logs deploy/mai --previous`, `git revert` | uptime monitor on `/readyz` and port 25 |
| 2 | Postgres pod crash | readiness probe | API 503, SMTP answers `451` at RCPT [observed `internal/mx/mx.go`], no mail lost | 0 / 2 min (crash recovery) | check `kubectl logs postgres-0`; if WAL corrupt, scenario 5 | CNPG later |
| 3 | Node reboot (kernel update, Hetzner maintenance) | uptime monitor | everything down 2 to 5 min | 0 / 5 min | none; verify Flux and pods Ready | unattended upgrades without auto reboot; reboot in a window |
| 4 | Disk full (80 GB disk, 71Gi of PVC requests, local-path does not enforce sizes) | none today | Postgres stops writing, ingest fails with `451` (no loss), backups fail | 0 / 1 h | `df -h /var/lib/rancher`, delete old images (`k3s crictl rmi --prune`), move blobs to S3 | alert at 80 % disk; blobs to Object Storage |
| 5 | Disk or node loss (hardware, accidental `hcloud server delete`) | uptime monitor (missing today) | total outage, data since last dump lost, inbound accepted mail lost | 24 h / hours | section 5.3 full restore with `deploy/dr/restore.sh` | 6 h dumps now, WAL archiving at T1, delete protection |
| 6 | Logical corruption (bug writes wrong data) | customer report, drill integrity checks | wrong data, spreads into every later backup | 24 h granularity / hours | restore a dump older than the bug into a scratch DB, repair with SQL (section 5.4) | PITR at T1, keep 30 days of dailies |
| 7 | Bad migration (`mai serve` runs migrations at start [observed `cmd/mai/main.go`]) | rollout fails or errors after deploy | schema broken, app down | 24 h / 1 to 2 h | `flux suspend`, scale `mai` to 0, restore last dump (section 5.3 steps 4 to 7), pin previous image SHA | take a dump in CI or a pre deploy Job before any migration; migrations only additive |
| 8 | Accidental deletion: namespace `mai` | Flux recreates objects, but PVCs and data are gone | total data loss on the node (local-path deletes the directory with the PV, reclaim policy Delete) | 24 h / hours | full restore onto the same server (restore.sh with `SERVER_NAME` of the existing server) | runbook rule already forbids it; set PV reclaim policy `Retain`; RBAC: the ops agent gets no delete on namespaces |
| 9 | Accidental deletion: PVC or tenant data (API `inbox.delete`, operator SQL) | audit log, customer | one tenant's data gone | 24 h / 1 h | section 5.4 single tenant restore | soft delete with 30 day purge; audit log alerts on bulk deletes |
| 10 | Ransomware or compromised operator token, including the Hermes ops agent (holds `HCLOUD_TOKEN`, `GITHUB_TOKEN` with Administration write, SSH) | unexpected Hetzner actions, Git force pushes, backups deleted | attacker can delete server, snapshots, automatic backups and, with in-cluster S3 keys, every backup object | total loss possible / days | revoke all tokens (Hetzner, GitHub, Brevo, S3), rotate SSH keys, restore from immutable copy into a new Hetzner project | Object Lock (compliance) bucket in a **separate Hetzner project** the agent cannot reach, Storage Box with snapshots, Git mirror, read only Hetzner token for the agent by default, Hetzner delete protection, approval gates (section 5.1). Now in place in git: the in cluster S3 key cannot delete (section 4.1) and Flux needs no GitHub PAT (AGENT_RUNBOOK section 0a) |
| 11 | Hetzner region (location) outage | uptime monitor, Hetzner status page | total outage; Primary IP and automatic backups are stuck in that location | 24 h / hours, if the bucket is in another location | restore.sh with `LOCATION=hel1` (or nbg1), new IP, commit new sslip.io names | bucket in a different location than the server; DNS with low TTL once owned |
| 12 | GitHub outage | Flux `Ready False`, CI red | no deploys; running cluster unaffected; restore.sh blocked (Flux bootstrap needs GitHub) | 0 / n/a | wait; emergency: `kubectl apply -k deploy/k8s/staging` from a local checkout or the mirror | nightly mirror; documented `flux bootstrap git` against the mirror |
| 13 | GHCR image deleted or GHCR outage | `ImagePullBackOff` on new pods | running pods fine until rescheduled; a restore cannot pull | 0 / 30 min | re-run the `release` workflow for the SHA in `deploy/k8s/staging/kustomization.yaml`; or build locally and `k3s ctr images import` | GHCR retention keeps the last 10 SHAs; pin third party images by digest; optional mirror to a second registry |
| 14 | Brevo outage or account suspended | `message.failed` events, `last_error` | outbound delayed. **Fixed (DR8):** temporary failures are retried for 5 days from `created_at` (`MAI_SEND_MAX_AGE`, default `120h`), at 1m, 5m, 30m, 1h, 2h, then every 3 h [`internal/core/workers.go`]; before, the sender gave up after about 8.6 h | 0 / Brevo's RTO | switch `MAI_SMTP_RELAY` to a second relay (or direct port 25 if Hetzner opened it), requeue failed messages with SQL | second relay credentials ready |
| 15 | Let's Encrypt outage or rate limit (after repeated rebuilds) | Caddy logs, TLS errors | API HTTPS fails for new certs; STARTTLS cert missing (mail falls back to plain, most senders still deliver) | 0 / up to 7 days for rate limits | restore `caddy-data` from backup; or Caddy's ZeroSSL fallback | back up `caddy-data`; LE allows 5 certificates per exact set of names per 7 days and 50 per registered domain per 7 days [source](https://letsencrypt.org/docs/rate-limits/) (2026-10) |
| 16 | Sending IP blacklisted (Spamhaus, Microsoft) | bounces with block codes, `mxtoolbox` check | outbound rejected; affects direct delivery only (Brevo uses its IPs) | 0 / days | keep Brevo relay as fallback; find the abusing tenant, suspend, request delisting | per org rate limits [observed `MAI_RATE_LIMIT_PER_SECOND`], outbound abuse scoring, separate IPs for free and paid later |
| 17 | Expired or revoked secret (GHCR PAT, Brevo key, S3 key, GitHub PAT used by Flux) | pull errors, relay auth errors, backup CronJob failing, Flux auth errors | depends; a silent S3 key expiry means **no backups** | 0 / 1 h | reissue in the console, `kubectl create secret ... --dry-run=client -o yaml \| kubectl apply -f -` | expiry calendar in the DR kit; dead man's switch catches backup failures |
| 18 | DNS failure (later, when the brand domain exists): registrar, provider outage, hijack | external monitor resolving MX and API | mail to customers' shared domain and API unreachable; senders retry for days | 0 / provider | secondary DNS provider, zone file in git | registrar lock, 2FA, two DNS providers, DNSSEC |
| 19 | Clock skew or expired k3s certificates (k3s rotates its certs on restart if < 90 days) | `kubectl` TLS errors | API server unreachable | 0 / 30 min | `systemctl restart k3s` | yearly reboot, chrony |

## 4. Backup options compared

Assumptions [estimate]: 20 GB database, `pg_dump -Fc` about 6 GB (mail text compresses roughly 3:1), 50 GB blobs (already compressed attachments, no gain), 1 to 5 GB of WAL per day at staging write rates. Hetzner Object Storage costs €6.49/month per account including 1 TB storage and 1 TB egress, extra about €8.70 per TB month [source](https://sliplane.io/blog/s3-providers-with-object-locking) (July 2026, list price [source](https://www.hetzner.com/storage/object-storage/) checked 2026-10-22), locations fsn1, nbg1, hel1, S3 compatible, versioning and Object Lock (compliance and governance), Object Lock only at bucket creation [source](https://docs.hetzner.com/storage/object-storage/howto-protect-objects/protect-object-lock-retention/) (2026-10). The base price is per account, so several buckets share it.

| Option | Cost / month at 20 GB + 50 GB | RPO | Restore time 20 GB [estimate] | Complexity | Licence | Notes |
|--------|------------------------------|-----|-------------------------------|------------|---------|-------|
| Current: nightly `pg_dump` + `rclone copy/sync` to Object Storage | €6.49 (30 × 6 GB + 50 GB = 230 GB, inside quota) | 24 h | 30 to 60 min | low | rclone MIT | logical, portable across versions, never tested |
| `pg_dump` every 1 to 6 h | €6.49 (6 hourly, 14 days: 56 × 6 GB = 340 GB) | 1 to 6 h | 30 to 60 min | low | same | each dump reads the whole DB: fine at 20 GB, not at 200 GB. **T0 pick: 6 h** |
| CloudNativePG + Barman Cloud plugin, WAL archiving to S3 | €6.49 (2 weekly base backups 12 GB + 14 days WAL ≤ 70 GB) | 5 min (`archive_timeout` default 5 min [source](https://cloudnative-pg.io/docs/1.30/appendixes/backup_barmanobjectstore/), 2026-10) | 20 to 40 min, PITR to any second in the window | medium: operator, CRDs, migration from the StatefulSet | Apache 2.0 (CNCF project) | in tree Barman support deprecated since 1.26, use the plugin [source](https://cloudnative-pg.io/plugin-barman-cloud/docs/0.7.0/migration/) (2026-10). Also gives replicas and failover. **T1 pick** |
| pgBackRest (sidecar or VM) | €6.49 | 1 min to 5 min | 15 to 30 min, parallel, delta restore | medium to high in k8s (no operator in our stack) | MIT | best tool outside Kubernetes; CNPG covers our needs inside |
| Hetzner automatic backups (disk image, daily, 7 slots) | 20 % of server price [source](https://hetsnap.com/blog/hetzner-cloud-backup-vs-snapshot-pricing-comparison) (2026): €1.70 for CX33 at €8.49 [source](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/) (price from 2026-06-15) | 24 h | 10 to 20 min (rebuild from image) | very low | n/a | seven slots, oldest dropped [source](https://docs.hetzner.com/cloud/servers/backups-snapshots/overview/) (2026-10); volumes not included; crash consistent only [estimate: Hetzner does not document application consistency]; same account and bound to the server, so no protection from token compromise or server deletion. **T0 extra layer** |
| Hetzner snapshots (manual or scheduled via API) | about €0.014 per GB month [source](https://betterstack.com/community/guides/web-servers/hetzner-cloud-review/) (2026): about €1 per 70 GB snapshot | when taken | 10 to 20 min | low | n/a | survive server deletion; take one before every risky change (already in AGENT_RUNBOOK step 2) |
| Velero + kopia file system backup of PVCs | €6.49 (dedup) | schedule | 30 to 90 min | medium | Apache 2.0 | restic uploader deprecated, backups via restic disallowed from 1.17 [source](https://velero.io/docs/v1.17/file-system-backup/) (2026-10). Copying a live Postgres data dir is not consistent; only worth it for blobs and manifests. **Not recommended** while git + dumps cover us |
| Hetzner Storage Box as second target (BX11 1 TB) | €3.20 [source](https://docs.aeroftp.app/providers/hetzner-storage-box) (2026, verify after the June 2026 price change) | follows the copy job | download speed bound | low | n/a | SFTP (port 23), rsync, Borg, restic, rclone, SMB, WebDAV, 10 snapshots on BX11 [source](https://www.hetzner.com/storage/storage-box/) (2026-10); different product and credentials from Object Storage. **T1 pick for copy 3** |
| Object Lock / immutable backups | €0 extra (storage of retained versions counts) | n/a | n/a | low, but create a new bucket | n/a | compliance mode cannot be shortened by anyone, governance can be bypassed with a special permission. **T0: new bucket, governance 14 days; T2: compliance 30 days** |
| Encryption: `age` (dumps) or `rclone crypt` (blobs) | €0 | n/a | + minutes | low | age BSD 3 clause, rclone MIT | encrypt with an age **public** key in the cluster; the private key never touches the cluster (password manager + paper in a safe). Losing the private key loses every backup: two custodial copies |

**3-2-1 mapping** [decision]:

| Rule | T0 staging | T1 / T2 |
|------|-----------|---------|
| 3 copies | live PVC, Object Storage bucket (locked, other location, encrypted), Hetzner automatic backup | live, Object Storage (WAL + base + blobs, versioned), Storage Box (nightly encrypted copy) |
| 2 media / systems | disk + S3 | S3 + SFTP box with its own snapshots |
| 1 off site and immutable | bucket in hel1 if the server is in fsn1/nbg1, in a separate Hetzner project, Object Lock | bucket in a separate Hetzner project, compliance lock; Storage Box snapshots |

### 4.1 Implemented T0 design: encrypted, add only, locked [decision, implemented]

**Encryption.** The upload container layers two rclone `crypt` remotes over the S3 remote (all configured through `RCLONE_CONFIG_*` environment variables, no config file): `dumps:` on `crypt/dumps` (file names encrypted, the date directories stay readable so a human can see what exists) and `blobs:` on `crypt/blobs` (file and directory names encrypted, so org ids and message ids do not leak). Content is encrypted with NaCl secretbox (XSalsa20 and Poly1305) keyed from `BACKUP_CRYPT_PASSWORD` and `BACKUP_CRYPT_SALT` [observed: rclone crypt documentation]. Hetzner never sees plaintext, DKIM private keys and webhook secrets included. The job refuses to upload when the keys are missing.

**Key custody.** `bootstrap.sh` generates both values with `openssl rand -hex 32` on the first deploy, stores them in secret `mai-backup` and prints them once in a banner; the founder keeps two offline copies (password manager entry plus paper). Reruns keep the existing values. Without them no backup can be restored: `restore.sh` requires them. Trade off versus the earlier `age` idea: `crypt` is symmetric, so the cluster can also decrypt; that adds nothing for an attacker who already has the cluster (the live database is there), and it lets the in cluster drill verify the real backups. A public key only scheme (`age`) would need a new tool and an off cluster drill; revisit at T2.

**Add only.** `rclone copy` everywhere, never `sync`, never `delete`; blobs with `--immutable` (an existing object that changed makes the job fail instead of overwriting it) and `--exclude *.tmp`; dumps under `crypt/dumps/<YYYY-MM-DD>/mai-<stamp>.dump` with `.counts` and `caddy-data-<stamp>.tar` beside them. Retention moved from the job to the bucket.

**Bucket.** New bucket created with Object Lock (only possible at creation [source](https://docs.hetzner.com/storage/object-storage/howto-protect-objects/protect-object-lock-retention/), 2026-10), default retention `GOVERNANCE` 30 days at T0 (`COMPLIANCE` from first paying customer), in a separate Hetzner project and another location than the server. Lifecycle: `crypt/dumps/` expires after 45 days, blobs are kept (a later GDPR erasure process must delete them with the vault key). Commands in `deploy/hetzner/AGENT_RUNBOOK.md` section 4b.

**Write only key.** Hetzner S3 keys reach every bucket of their own project and nothing else; a bucket policy can grant one key of another project specific actions [source](https://docs.hetzner.com/storage/object-storage/faq/s3-credentials/), 2026-10. The cluster's key lives in the staging project and is granted `ListBucket`, `GetBucketLocation`, `GetObject`, `GetObjectVersion`, `PutObject` and the multipart actions on the vault bucket only: no `DeleteObject`, no `DeleteObjectVersion`, no retention, lifecycle or policy changes. With versioning (implied by Object Lock) an overwrite only adds a version. `NotAction` is not supported by Hetzner, so the policy is an allow list. Open item: verify on the first run that Hetzner accepts rclone's uploads into a locked bucket (some S3 servers require `Content-MD5` there).

**Read only dump role.** `pg_dump` runs as `mai_backup` (`BYPASSRLS` because every tenant table has `FORCE ROW LEVEL SECURITY`, `pg_read_all_data`, `default_transaction_read_only = on`), created idempotently by `bootstrap.sh` (roles are cluster wide, no migration needed). The job falls back to `postgres` when `BACKUP_PG_USER` is not in the secret. The dump and the row counts share one exported snapshot (`pg_export_snapshot` plus `pg_dump --snapshot`), so the drill can demand an exact match. The DB roles work (`mai_owner`, `mai_app`) does not affect this: the drill restores with `--no-owner --no-privileges`, and `restore.sh` takes the database owner from `DB_OWNER` (default `mai_app`).

## 5. Disaster recovery plan

### 5.1 Roles and approval gates

| Role | Can do alone | Needs the founder's explicit "yes" |
|------|--------------|-----------------------------------|
| Founder | everything, declares disasters | n/a |
| Ops agent (Hermes) | read status, logs, run `restore-drill` and manual backups, create a new server, run `restore.sh` up to step 3, draft status updates | step 5 of restore.sh (database replacement), moving the Primary or Floating IP, deleting anything, rotating tokens, publishing a status update |
| Tokens | the agent's default Hetzner token is **read only**; a read/write token is issued for the duration of a declared incident and revoked after | |

### 5.2 Declaration and communication

Declare a disaster when any is true: data loss is confirmed or likely; the API or port 25 is down for more than 30 minutes with no fix in sight; the server or its disk is gone; a token compromise is suspected. Then:

1. Open an incident note (time, symptoms, decision). Freeze deploys: `flux suspend kustomization mai` if the cluster still answers.
2. Status page (Upptime on GitHub Pages, free, already planned in doc 06): "Investigating" within 15 minutes, then every 60 minutes.
3. Customers (T1 and later): email from the founder with what is affected, whether data since a given time may be lost, and that inbound mail is retried by senders for days. After recovery: a written postmortem within 5 days.

### 5.3 Full restore to a NEW Hetzner server

Automated in [`deploy/dr/restore.sh`](../../deploy/dr/restore.sh) (idempotent, no prompts when the variables are set, **untested**). The manual equivalent:

1. **Preconditions.** The founder's offline `BACKUP_CRYPT_PASSWORD` and `BACKUP_CRYPT_SALT`. Backups reachable and decryptable: with the `store`, `dumps` and `blobs` remotes configured as in `deploy/k8s/base/backup.yaml`, `rclone lsf -R --include "mai-*.dump" dumps: | sort | tail`. Decide the IP: if the old server is dead but its location is healthy, keep the Primary IP (the holder must be powered off before it can be moved [source](https://docs.hetzner.com/cloud/servers/primary-ips/faq/), 2026-10), which keeps the sslip.io names; otherwise a new IP and a git commit of new names in `deploy/k8s/staging/kustomization.yaml`.
2. **Create the server.**
   `hcloud server create --name mai-dr-1 --type cx33 --image ubuntu-24.04 --location fsn1 --ssh-key $SSH_KEY_NAME --firewall mai-staging [--primary-ipv4 <id>]`, then `hcloud server set-rdns mai-dr-1 --ip <ip> --hostname mx.<ip-dashed>.sslip.io`.
3. **Bootstrap.** `scp deploy/hetzner/bootstrap.sh root@<ip>:` and run it non interactively as in AGENT_RUNBOOK section 4. It installs k3s and Flux, recreates `mai-secrets` (new DB passwords; roles are not part of `pg_dump`, the init script recreates `mai_app`), `mai-backup`, `ghcr-pull`, and seeds tester accounts that are discarded in step 5.
4. **Freeze.** `flux suspend kustomization mai`; `kubectl -n mai scale deploy/mai --replicas=0` (Flux would scale it back otherwise; `mai serve` would also run migrations against the empty DB).
5. **Restore Postgres into the StatefulSet.** Download the newest dump onto the Postgres volume with a one off Job (restore.sh does this), then:
   ```bash
   kubectl -n mai exec -i postgres-0 -- sh -ec '
     pg_restore --list /var/lib/postgresql/data/restore.dump >/dev/null
     psql -U postgres -d postgres -c "drop database if exists mai with (force)"
     psql -U postgres -d postgres -c "create database mai owner mai_app"
     pg_restore -U postgres -d mai --exit-on-error /var/lib/postgresql/data/restore.dump
     rm /var/lib/postgresql/data/restore.dump'
   ```
   The download Job decrypts on the fly (`rclone copyto dumps:<day>/mai-<stamp>.dump`); `bootstrap.sh` must have been given the same crypt keys. restore.sh then compares the row count of every table with the `.counts` file stored next to the dump and stops on any difference.
6. **Restore blobs.** Job mounting `mai-blobs` as uid 65532: `rclone copy --transfers 16 blobs: /blobs` (decrypts on the fly). 50 GB inside Hetzner's network takes about 10 to 20 minutes [estimate]. Optional: `caddy-data` the same way to skip certificate issuance.
7. **Resume.** `flux resume kustomization mai && flux reconcile kustomization mai --with-source`, `kubectl -n mai rollout status deploy/mai`.
8. **Verify.**
   * `/readyz` returns 204 in cluster and on `https://api.<ip-dashed>.sslip.io/readyz`.
   * Row counts (`organizations`, `inboxes`, `messages`, `attachments`, `events`, `audit_log`, `schema_migrations`) match the `.counts` file written next to the dump exactly (same snapshot; restore.sh checks every table) until the platform is resumed.
   * Newest `messages.raw_key` exists in `/var/lib/mai/blobs`; `GET /v0/inboxes/{inbox}/messages/{id}/raw` with a customer approved or tester key returns the `.eml`.
   * Send and receive smoke test (AGENT_RUNBOOK section 5) with a tester key.
   * Port 25 banner from outside: `nc -v <ip> 25`, STARTTLS: `openssl s_client -starttls smtp -connect <ip>:25 -servername mx.<ip-dashed>.sslip.io`.
9. **Switch traffic.** With a Floating IP: `hcloud floating-ip assign mai-edge mai-dr-1` (seconds, no DNS change). With the brand domain later: lower TTL to 300 s ahead of time, then update A/AAAA/MX. Take a fresh backup right away.
10. **Known side effects.** Outbound messages that were `queued` or `sending` in the dump are sent again (possible duplicates); pending webhook deliveries are retried (consumers dedupe on `event_id`); idempotency keys roll back to the dump time; mail accepted after the dump is gone.

Expected RTO with practice [estimate]: 15 min server and bootstrap, 5 min Flux, 20 to 40 min database, 15 min blobs, 15 min checks: **about 1.5 h**; staging data is far smaller today, so much less.

### 5.4 Partial restore: one tenant

Never restore over production. Restore into a temporary database, then copy only the tenant's missing rows.

```bash
ORG=<org uuid>; DUMP=mai-2026-10-21T0217Z.dump   # decrypted copy: rclone copyto dumps:2026-10-21/$DUMP $DUMP
# 1. temporary database in the same Postgres (or use the restore-drill pod)
kubectl -n mai exec postgres-0 -- psql -U postgres -c "create database mai_restore owner mai_app"
kubectl -n mai cp $DUMP postgres-0:/var/lib/postgresql/data/$DUMP
kubectl -n mai exec postgres-0 -- pg_restore -U postgres -d mai_restore --exit-on-error /var/lib/postgresql/data/$DUMP
# 2. copy the tenant's rows into an import schema in production, in FK order
for t in organizations domains api_keys inboxes threads messages attachments webhooks audit_log; do
  col=org_id; [ $t = organizations ] && col=id
  kubectl -n mai exec postgres-0 -- psql -U postgres -d mai -qc \
    "create schema if not exists dr_import; drop table if exists dr_import.$t; create table dr_import.$t (like public.$t)"
  kubectl -n mai exec postgres-0 -- sh -c \
    "psql -U postgres -d mai_restore -c \"\\copy (select * from $t where $col = '$ORG') to stdout\" | psql -U postgres -d mai -c \"\\copy dr_import.$t from stdin\""
done
# 3. review counts in dr_import, then insert what is missing (superuser bypasses RLS)
for t in organizations domains api_keys inboxes threads messages attachments webhooks audit_log; do
  kubectl -n mai exec postgres-0 -- psql -U postgres -d mai -c "insert into public.$t select * from dr_import.$t on conflict do nothing"
done
# 4. blobs of this tenant only (keys are org prefixed)
rclone copy --include "raw/$ORG/**" --include "att/$ORG/**" blobs: <job mounting mai-blobs>   # filters see decrypted names
# 5. clean up
kubectl -n mai exec postgres-0 -- psql -U postgres -c "drop database mai_restore" -c "\c mai" -c "drop schema dr_import cascade"
```

`events` and `webhook_deliveries` are left out on purpose so restored data does not fire webhooks again; `audit_log` gets a manual row `operator / tenant.restore`. Thread counters (`message_count`) may need a recount query.

## 6. Network design for resilience

| Topic | Recommendation [decision] | Cost |
|-------|---------------------------|------|
| Stable address | **T0:** disable auto delete on the Primary IPv4 and enable delete protection (`hcloud primary-ip update <id> --auto-delete=false`, `hcloud primary-ip enable-protection <id> delete`, same for the server). **T1:** a Floating IPv4 `mai-edge` for MX and API, configured on the node with netplan (restore.sh writes it); it moves in seconds between servers in the same network zone. Hetzner LB is not a fit for SMTP: it proxies TCP and hides the client IP that rspamd and SPF need, unless PROXY protocol is added to the Go `mx` | Primary IPv4 €0.50, Floating IPv4 €3.00, Floating IPv6 €1.00 [source](https://docs.hetzner.com/general/infrastructure-and-availability/ipv4-pricing/) (2026-10); LB11 €7.49 [source](https://costgoat.com/pricing/hetzner) (Oct 2026) |
| Backup MX | **Not before the brand domain exists** (sslip.io names cannot carry MX records for customer domains anyway). Senders retry 4xx and unreachable hosts: retry interval at least 30 minutes, give up after at least 4 to 5 days [RFC 5321 §4.5.4.1](https://www.rfc-editor.org/info/rfc5321/) (2008), Postfix default `maximal_queue_lifetime` 5d [source](https://www.postfix.org/postconf.5.html) (2026-10). A backup MX that only answers 4xx adds nothing. A real backup MX (MX 20, lower preference, used only when MX 10 fails, RFC 5321 §5.1) must queue: a second small server in another location running the Go `mx` in a new "relay" mode, with a recipient list synced from Postgres every 5 minutes (reject unknown recipients, no backscatter), queueing to local disk and forwarding to MX 10 when it is back. Backup MXs attract spam because spammers target them: same rspamd checks. **T2 decision** | smallest cost optimized server about €6 [estimate from [Hetzner](https://www.hetzner.com/cloud/cost-optimized/), Oct 2026] |
| Hetzner Cloud Firewall | keep the AGENT_RUNBOOK rules (22 from admin IP only, 25, 80, 443, ICMP). Add IPv6 rules once AAAA records exist. Block 6443 (k3s API) from the internet explicitly; manage through SSH or the private network | free |
| Kubernetes NetworkPolicy | default deny ingress and egress in `mai`, explicit allows; public egress excludes private, link local (Hetzner metadata `169.254.169.254`) and CGNAT ranges so a customer webhook URL cannot reach cluster internals (SSRF). See [`deploy/dr/networkpolicy-default-deny.yaml`](../../deploy/dr/networkpolicy-default-deny.yaml). Enforced by the k3s embedded network policy controller | free |
| Private network | at 3 nodes: Hetzner Cloud Network `10.0.0.0/16`, k3s `--node-ip` on the private interface, flannel over it (`--flannel-iface`), etcd and kubelet ports only on the private network | free |
| IPv6 | each server has a /64 for free [source](https://docs.hetzner.com/general/infrastructure-and-availability/ipv4-pricing/) (2026-10). Inbound AAAA for API and MX is cheap; outbound mail over IPv6 needs its own PTR and reputation, so keep outbound on IPv4 (or Brevo) until volume justifies it | free |
| Edge rate limiting | API: per org limit already in the gateway; add per IP limits in Caddy (needs the `rate_limit` plugin in the landing image) or Cloudflare free in front of the API once the brand domain exists. SMTP: per IP connection and recipient limits in the Go `mx` (code change), rspamd greylisting for unknown senders | free |
| DDoS at zero budget | rely on Hetzner's included network level DDoS filtering [estimate: advertised by Hetzner, not verified for this doc]; Cloudflare proxy for HTTPS only (it cannot proxy port 25 on the free plan); keep the origin IP of the API distinct from the MX IP at T2 so an HTTP flood does not take mail down | free |

## 7. Kubernetes HA roadmap

| Stage | Topology | Storage | Postgres | Blobs | Other | Cost / month [estimate] |
|-------|----------|---------|----------|-------|-------|-------------------------|
| T0 now | 1 × CX33, k3s server with SQLite (kine); etcd snapshots do not apply to SQLite: cluster state comes from git, the k3s datastore is not backed up on purpose | local-path | StatefulSet, dumps every 6 h | PVC, `rclone copy` every 6 h | Hetzner automatic backups, monitors | CX33 €8.49 + IPv4 €0.50 + Object Storage €6.49 + backups €1.70 = **about €17** |
| T1 first customers | still 1 node, but stateless app: blobs in S3 | local-path for Postgres only | CloudNativePG, 1 instance, Barman Cloud plugin WAL archiving, weekly base backup, 14 day PITR | Object Storage bucket (versioning), `internal/blob` S3 store | Floating IP, Storage Box copy, status page | + Floating IP €3 + Storage Box €3.20 = **about €23** |
| T2 paid | 3 × CX33 k3s servers with embedded etcd (odd count for quorum [source](https://docs.k3s.io/datastore/ha-embedded), 2026-10), private network, etcd snapshots to S3 (`--etcd-s3 --etcd-s3-endpoint ... --etcd-snapshot-schedule-cron "0 */6 * * *"` [source](https://docs.k3s.io/cli/etcd-snapshot), 2026-10) | Hetzner CSI volumes for Postgres (€0.057/GB month [source](https://betterstack.com/community/guides/web-servers/hetzner-cloud-review/), 2026); Longhorn (synchronous replicas across nodes [source](https://longhorn.io/docs/1.13.0/concepts/), 2026-10) is not needed when CNPG replicates at the database level | CNPG primary + 1 replica on different nodes, automatic failover, `podAntiAffinity` | S3 only, app pods stateless | `mai` 2 replicas once the SMTP edge and workers tolerate it (`for update skip locked` already makes workers safe [observed]), PodDisruptionBudget `minAvailable: 1` for mai and CNPG, `topologySpreadConstraints` on `kubernetes.io/hostname`, Floating IP failover script or keepalived, optional backup MX | 3 × €8.99 + volumes 2 × 40 GB €4.60 + Object Storage €6.49 + Floating IP €3 + Storage Box €3.20 + backup MX €6 = **about €50**; + LB11 €7.49 if the API moves behind a load balancer |

Notes: the current `Recreate` strategy and `ReadWriteOnce` blob volume are what pin `mai` to one node; moving blobs to S3 is the single change that unlocks everything else. Port 25 through k3s ServiceLB with `externalTrafficPolicy: Local` only answers on nodes that run a `mai` pod, which is what the Floating IP must follow.

## 8. Testing

**Weekly restore drill** (automated, in GitOps): [`deploy/k8s/base/restore-drill.yaml`](../../deploy/k8s/base/restore-drill.yaml), Mondays 04:43 UTC. Downloads and decrypts the newest dump and its `.counts`, fails if the dump is older than 30 h or smaller than 4 KB, restores it with `--exit-on-error --no-owner --no-privileges` into a scratch Postgres inside the pod (unix socket only, never the production database; foreign keys are validated during the load), then fails unless the row count of every table equals the count recorded at backup time from the same snapshot, `schema_migrations` and `organizations` are not empty, and every table with an `org_id` column still has `FORCE ROW LEVEL SECURITY`. Finally it decrypts 20 recent raw messages and 20 attachments from the blob backup and fails on any missing or empty one. Requests stay small for the CX33 (largest step 100m CPU, 256Mi, limit 1Gi). Only a full pass pings the dead man's switch, so silence alerts; the failed Job also stays visible for 7 days.

**Quarterly game day checklist** (founder + ops agent, 2 hours, staging):

- [ ] Announce on the status page as maintenance.
- [ ] Take a Hetzner snapshot of the live server.
- [ ] Run `deploy/dr/restore.sh` with `SERVER_NAME=mai-gameday` in another location, no IP move.
- [ ] Time each step; compare with the RTO target; record RPO from the dump timestamp.
- [ ] Verification list of section 5.3 step 8, plus a partial restore of one tester org (section 5.4) into the game day server.
- [ ] Simulate a token compromise on paper: list every token, who holds it, how to revoke it in under 15 minutes.
- [ ] Delete the game day server; write a one page log in `docs/dr-log/` with date, durations, failures, fixes.

**Metrics and alerts** (minimal, free):

| Signal | How | Alert |
|--------|-----|-------|
| Backup success and age | healthchecks.io ping from the backup CronJob (`/start`, success, `/fail`), period 6 h, grace 1 h; Hobbyist plan free for 20 checks [source](https://healthchecks.io/pricing/) (2026-10) | email + push when a ping is missing or `/fail` |
| Backup size anomaly | `.counts` and dump size in the success ping body; healthchecks keeps the last 100 bodies; later a Prometheus rule `abs(delta) > 50 %` | weekly manual glance at T0 |
| Restore drill result | its own healthchecks.io check, period 7 days, grace 1 day | missing ping |
| API and SMTP up | Upptime (GitHub Actions, free) or Uptime Kuma (MIT, self hosted on a different machine: monitoring on the same node dies with it) checking `https://api.../readyz` and TCP 25 every 5 minutes | 2 failures in a row |
| Disk usage | node exporter later; at T0 a one line cron on the host posting `df` to a healthchecks.io check that fails above 80 % | `/fail` ping |
| Outbound failures | count of `message.failed` per hour (SQL in the drill or a metric later) | > 10 per hour |

## 9. Files (GitOps manifests and `deploy/dr/`)

| File | Purpose | Validated |
|------|---------|-----------|
| [`deploy/k8s/base/restore-drill.yaml`](../../deploy/k8s/base/restore-drill.yaml) (adopted from the former `deploy/dr/restore-drill-cronjob.yaml`) | weekly restore drill, exact row counts, RLS and blob checks, dead man's switch | kustomize + kubeconform strict; steps run locally against Postgres 16 and rclone 1.75.2; not yet run on the cluster |
| [`deploy/k8s/base/backup.yaml`](../../deploy/k8s/base/backup.yaml) (adopted from the former `deploy/dr/backup-cronjob-v2.yaml`) | 6 hourly validated dumps, encrypted, add only, caddy-data, dead man's switch; retention by bucket lifecycle | kustomize + kubeconform strict; dump and encrypted upload run locally; not yet run against Hetzner Object Storage |
| [`networkpolicy-default-deny.yaml`](../../deploy/dr/networkpolicy-default-deny.yaml) | proposal: default deny + allow list for caddy, mai, postgres, rspamd, backup, restore drill, DNS, SMTP and HTTPS egress | YAML parses; not applied |
| [`restore.sh`](../../deploy/dr/restore.sh) | full restore to a new server, idempotent, non interactive with env vars, encrypted layout, exact row count check, protections at the end | `bash -n`; generated Job manifest passes kubeconform; never run end to end |
| [`README.md`](../../deploy/dr/README.md) | what is in GitOps, what stays in `deploy/dr` | n/a |

## 10. Decisions for the founder

| # | Decision | Options | Recommendation |
|---|----------|---------|----------------|
| DR1 | Backup encryption and key custody | none; `age` with one key; `age` with two recipients (founder key + offline paper key); rclone `crypt` | **done (2026-10-22): rclone `crypt`** (no new tool, lets the in cluster drill verify); keys generated by `bootstrap.sh`, two offline copies with the founder (section 4.1); revisit `age` at T2 |
| DR2 | Where immutable backups live | same Hetzner project; separate Hetzner project; separate provider | separate Hetzner project now (free), second provider at T2 |
| DR3 | Object Lock mode and duration | governance 14 d; governance 30 d; compliance 30 d | governance 30 d at T0 (the job no longer deletes, so the lock no longer has to be shorter than a job retention), compliance 30 d from the first paying customer; dumps expire by lifecycle after 45 d |
| DR4 | Postgres engine path | stay on StatefulSet + dumps; CNPG at T1; managed (Supabase, D9) | CNPG at T1 (keeps self hosting decision D9) |
| DR5 | Floating IP now or at T1 | now (€3); T1 | T1, but disable Primary IP auto delete and enable delete protection today (commands in AGENT_RUNBOOK section 1b) |
| DR6 | Ops agent permissions | as today (read/write Hetzner, GitHub admin); read only by default with incident tokens | read only by default; documented in AGENT_RUNBOOK section 0a: no GitHub PAT (deploy key), dedicated Hetzner project, read/write token deleted after each deploy, no token for the backup vault project |
| DR7 | Backup MX | none (rely on sender retries); relay mode in Go at T2 | none until T2 |
| DR8 | Outbound give up time | keep about 8.6 h; extend to 4 to 5 days per RFC 5321 | **done (2026-10-22):** 5 days from `created_at`, configurable with `MAI_SEND_MAX_AGE` (default `120h`); retries at 1m, 5m, 30m, 1h, 2h, then every 3 h, the last one at the end of the window; 5xx still fails at once; see section 3 #14 |
| DR9 | Retention | 14 days of 6 hourly; plus monthly dumps for 12 months | add monthly for 12 months once customers exist (GDPR: document it in doc 08) |
