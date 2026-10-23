# deploy/dr: disaster recovery tools and the remaining proposal

The backup and the weekly restore drill now live in GitOps, in
[`deploy/k8s/base`](../k8s/base), and Flux applies them:

| Manifest | What |
|----------|------|
| [`backup.yaml`](../k8s/base/backup.yaml) | CronJob every 6 hours: `pg_dump` (read only role `mai_backup`) with exact row counts from the same snapshot, blobs and `caddy-data`, encrypted with rclone `crypt` and uploaded with `rclone copy` only (no sync, no delete, `--immutable` blobs) |
| [`restore-drill.yaml`](../k8s/base/restore-drill.yaml) | CronJob every Monday: decrypts the newest dump, restores it into a scratch Postgres inside the pod, compares every table's row count with the backup, checks RLS and a sample of blobs, pings a dead man's switch only on a full pass |

The reasoning, bucket setup (Object Lock, write only key) and runbooks are in
[docs/agentmail-re/14-disaster-recovery-backup.md](../../docs/agentmail-re/14-disaster-recovery-backup.md).

What stays here:

| File | What | How to use it |
|------|------|---------------|
| `restore.sh` | Full restore to a new Hetzner server: create server (optionally taking over the Primary IP), `bootstrap.sh` with the same crypt keys, freeze Flux, decrypt and restore the newest dump into the StatefulSet, verify row counts against the backup, restore blobs, resume, verify, enable backups and protection | From the operator machine: export the variables listed at the top of the script (including `BACKUP_CRYPT_PASSWORD` and `BACKUP_CRYPT_SALT` from the founder's offline copy), then `bash deploy/dr/restore.sh`. Rerunnable; it skips finished steps. Not yet run end to end |
| `networkpolicy-default-deny.yaml` | **Proposal, not applied.** Default deny ingress and egress for namespace `mai`, plus allow rules for the real flows (caddy to mai 8080, mai to postgres 5432 and rspamd 11333, backup to postgres, DNS, SMTP 25/587 out, HTTPS 443 out to public addresses only, restore drill to 443 only) | `kubectl -n mai apply -f deploy/dr/networkpolicy-default-deny.yaml`, then check every pod is Ready, run the smoke test in `deploy/hetzner/AGENT_RUNBOOK.md` section 5, a manual backup and a manual restore drill. Roll back with `kubectl -n mai delete netpol -l dr.proposal=default-deny`. Once it passes, move it into `deploy/k8s/base` (replacing `networkpolicy.yaml`) |

## Monitor URLs (optional, recommended)

`bootstrap.sh` keeps these keys in secret `mai-backup` when they exist. Add
them without touching the other keys (healthchecks.io Hobbyist is free for 20 checks):

```bash
kubectl -n mai patch secret mai-backup --type merge -p "{\"stringData\":{
  \"HC_BACKUP_URL\":\"https://hc-ping.com/<uuid-backup>\",
  \"HC_RESTORE_DRILL_URL\":\"https://hc-ping.com/<uuid-drill>\"}}"
```

Set the backup check period to 6 hours with a 1 hour grace, and the drill
check to 7 days with a 1 day grace.

## Manual runs

```bash
kubectl -n mai create job --from=cronjob/backup backup-manual-$(date +%s)
kubectl -n mai create job --from=cronjob/restore-drill restore-drill-manual-$(date +%s)
kubectl -n mai logs -f job/<name> --all-containers
```
