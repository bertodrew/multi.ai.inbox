# deploy/dr: disaster recovery proposals

Everything in this directory is a **proposal**. Nothing here is referenced by
`deploy/k8s` or `deploy/flux`, so Flux does not apply it. None of it has been run
against the cluster yet. The reasoning, costs and runbooks are in
[docs/agentmail-re/14-disaster-recovery-backup.md](../../docs/agentmail-re/14-disaster-recovery-backup.md).

| File | What | How to try it |
|------|------|---------------|
| `networkpolicy-default-deny.yaml` | Default deny ingress and egress for namespace `mai`, plus allow rules for the real flows (caddy to mai 8080, mai to postgres 5432 and rspamd 11333, backup to postgres, DNS, SMTP 25/587 out, HTTPS 443 out to public addresses only) | `kubectl -n mai apply -f deploy/dr/networkpolicy-default-deny.yaml`, then check every pod is Ready, run the smoke test in `deploy/hetzner/AGENT_RUNBOOK.md` section 5, a manual backup and a manual restore drill. Roll back with `kubectl -n mai delete netpol -l dr.proposal=default-deny` |
| `restore-drill-cronjob.yaml` | Monthly CronJob: newest dump into a scratch Postgres inside the pod, integrity queries, sample blob check, then a dead man's switch ping | Add `HC_RESTORE_DRILL_URL` to secret `mai-backup` (below), `kubectl -n mai apply -f deploy/dr/restore-drill-cronjob.yaml`, then `kubectl -n mai create job --from=cronjob/restore-drill drill-$(date +%s)` and read the logs with `--all-containers` |
| `backup-cronjob-v2.yaml` | Replacement for `deploy/k8s/base/backup.yaml`: every 6 hours, validated dump, blobs with `copy` not `sync`, caddy-data included, dead man's switch | Copy over `deploy/k8s/base/backup.yaml` in a PR once reviewed (Flux would revert a manual `kubectl apply` of the same name within 5 minutes) |
| `restore.sh` | Full restore to a new Hetzner server: create server (optionally taking over the Primary IP), `bootstrap.sh`, freeze Flux, restore the dump into the StatefulSet, restore blobs, resume, verify | From the operator machine: export the variables listed at the top of the script, then `bash deploy/dr/restore.sh`. Rerunnable; it skips finished steps |

## Adding the monitor URLs to the existing secret

`bootstrap.sh` creates `mai-backup` without these keys. Add them without
touching the S3 keys (healthchecks.io Hobbyist is free for 20 checks):

```bash
kubectl -n mai patch secret mai-backup --type merge -p "{\"stringData\":{
  \"HC_BACKUP_URL\":\"https://hc-ping.com/<uuid-backup>\",
  \"HC_RESTORE_DRILL_URL\":\"https://hc-ping.com/<uuid-drill>\"}}"
```

Set the backup check period to 6 hours with a 1 hour grace, and the drill
check to 31 days with a 1 day grace.

## Adopting a proposal into GitOps

1. Copy the file into `deploy/k8s/base/` (or replace `networkpolicy.yaml`/`backup.yaml`).
2. Add it to `deploy/k8s/base/kustomization.yaml` `resources`.
3. Open a PR; after merge Flux applies it within 5 minutes.
4. Run the checks in the table above and record the result in the DR log (doc 14, section 8).
