#!/usr/bin/env bash
# Full restore of multi.ai.inbox onto a NEW Hetzner Cloud server, from git
# (Flux) + the encrypted Object Storage backups written by
# deploy/k8s/base/backup.yaml (rclone crypt, layout crypt/dumps/<day>/ and
# crypt/blobs/). Operator tool: not run by CI or Flux.
#
# Runs on the operator's machine (founder laptop or the ops agent host) with
# hcloud, ssh, scp, jq and curl installed. Idempotent: every step checks state
# first and the data steps leave markers in /root/.mai-restore on the server,
# so a rerun continues where it stopped. Prompts nothing when the variables
# below are set; without a TTY a missing variable is a hard error.
#
# Required:
#   HCLOUD_TOKEN, SSH_KEY_NAME
#   GHCR_TOKEN, BREVO_LOGIN, BREVO_KEY                      (same as bootstrap.sh)
#   BACKUP_S3_ENDPOINT, BACKUP_S3_BUCKET, BACKUP_S3_ACCESS_KEY, BACKUP_S3_SECRET_KEY
#   BACKUP_CRYPT_PASSWORD, BACKUP_CRYPT_SALT   the founder's offline copy of the crypt keys;
#                 without them nothing can be decrypted
#   RESTORE_CONFIRM=yes   (acknowledges that the database on the target server is replaced)
# Optional:
#   SERVER_NAME   default mai-dr-<yyyymmdd>
#   SERVER_TYPE   default cx33          LOCATION   default fsn1 (fsn1, nbg1, hel1)
#   FIREWALL      default mai-staging   (attached if it exists)
#   PRIMARY_IPV4  name or id of an existing Primary IP to move to the new server
#                 (the server holding it must be powered off; keeps 2.28.99.252 and the sslip.io names)
#   FLOATING_IP   name or id of a Floating IP to point at the new server
#   MX_HOSTNAME   PTR to set, e.g. mx.2-28-99-252.sslip.io
#   API_HOST      e.g. api.2-28-99-252.sslip.io, for the external /readyz check
#   DUMP          dump path under crypt/dumps, e.g. 2026-10-21/mai-2026-10-21T0217Z.dump (default: newest)
#   DB_OWNER      role that owns database mai (default mai_owner, created by postgres-init.sh)
#   GITHUB_TOKEN  only for the legacy PAT bootstrap; by default bootstrap.sh uses a deploy key
#   FLUX_DEPLOY_KEY_FILE  local private key already registered as a GitHub deploy key; copied to
#                 the server so bootstrap.sh does not stop to ask for a new one
#   SKIP_BLOBS=1  restore the database only
#   SMOKE_API_KEY tester API key for the send/receive smoke test (staging only)
#   REPO_DIR      repository checkout holding deploy/hetzner/bootstrap.sh (default: this script's repo)
#
# NOT TESTED end to end. Dry run it on a throwaway server before relying on it.
set -euo pipefail

REPO_DIR=${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
SERVER_NAME=${SERVER_NAME:-mai-dr-$(date +%Y%m%d)}
SERVER_TYPE=${SERVER_TYPE:-cx33}
LOCATION=${LOCATION:-fsn1}
IMAGE=${IMAGE:-ubuntu-24.04}
FIREWALL=${FIREWALL:-mai-staging}
NS=mai

log() { printf '\n==> %s\n' "$*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

need() { # need VAR "description"
  if [[ -n "${!1:-}" ]]; then return; fi
  [[ -t 0 ]] || die "missing environment variable $1 ($2)"
  local v; read -rsp "$2 ($1): " v; echo; printf -v "$1" '%s' "$v"; export "${1?}"
}

for bin in hcloud ssh scp jq curl; do command -v "$bin" >/dev/null || die "$bin not installed"; done
need HCLOUD_TOKEN "Hetzner Cloud API token"; export HCLOUD_TOKEN
need SSH_KEY_NAME "SSH key name in the Hetzner project"
BOOT_VARS="GHCR_TOKEN BREVO_LOGIN BREVO_KEY BACKUP_S3_ENDPOINT BACKUP_S3_BUCKET BACKUP_S3_ACCESS_KEY BACKUP_S3_SECRET_KEY BACKUP_CRYPT_PASSWORD BACKUP_CRYPT_SALT"
for v in $BOOT_VARS; do
  need "$v" "value for bootstrap.sh"
done
if [[ -n "${GITHUB_TOKEN:-}" ]]; then BOOT_VARS+=" GITHUB_TOKEN"; fi
DB_OWNER=${DB_OWNER:-mai_owner}
[[ -f "$REPO_DIR/deploy/hetzner/bootstrap.sh" ]] || die "bootstrap.sh not found under REPO_DIR=$REPO_DIR"
if [[ "${RESTORE_CONFIRM:-}" != yes ]]; then
  [[ -t 0 ]] || die "set RESTORE_CONFIRM=yes to replace the database on $SERVER_NAME"
  read -rp "Restore backups onto server $SERVER_NAME (its database is replaced)? Type yes: " RESTORE_CONFIRM
  [[ "$RESTORE_CONFIRM" == yes ]] || die "aborted"
fi

# ---------------------------------------------------------------- 1. server
log "1. Server $SERVER_NAME ($SERVER_TYPE, $LOCATION)"
if hcloud server describe "$SERVER_NAME" >/dev/null 2>&1; then
  echo "exists, reusing"
else
  create_args=(--name "$SERVER_NAME" --type "$SERVER_TYPE" --image "$IMAGE" --location "$LOCATION" --ssh-key "$SSH_KEY_NAME")
  if hcloud firewall describe "$FIREWALL" >/dev/null 2>&1; then create_args+=(--firewall "$FIREWALL"); else echo "WARN: firewall $FIREWALL not found, create it (AGENT_RUNBOOK.md section 3)"; fi
  if [[ -n "${PRIMARY_IPV4:-}" ]]; then
    holder=$(hcloud primary-ip describe "$PRIMARY_IPV4" -o json | jq -r '.assignee_id // empty')
    if [[ -n "$holder" ]]; then
      status=$(hcloud server describe "$holder" -o json | jq -r .status)
      [[ "$status" == off ]] || die "Primary IP $PRIMARY_IPV4 is held by server $holder (status $status). Power it off first: hcloud server poweroff $holder"
      hcloud primary-ip unassign "$PRIMARY_IPV4"
    fi
    create_args+=(--primary-ipv4 "$PRIMARY_IPV4")
  fi
  hcloud server create "${create_args[@]}"
fi
IP=$(hcloud server ip "$SERVER_NAME")
echo "server IP: $IP"

if [[ -n "${MX_HOSTNAME:-}" ]]; then
  hcloud server set-rdns "$SERVER_NAME" --ip "$IP" --hostname "$MX_HOSTNAME"
fi

# ---------------------------------------------------------------- 2. ssh
log "2. Waiting for SSH"
ssh-keygen -R "$IP" >/dev/null 2>&1 || true
for _ in $(seq 1 60); do
  ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=accept-new "root@$IP" true 2>/dev/null && break
  sleep 5
done
rssh() { ssh -o BatchMode=yes "root@$IP" "$@"; }
rssh true || die "SSH to $IP failed"
rssh 'mkdir -p /root/.mai-restore'
done_step() { rssh "test -f /root/.mai-restore/$1"; }
mark_step() { rssh "touch /root/.mai-restore/$1"; }

if [[ -n "${FLOATING_IP:-}" ]]; then
  log "2b. Floating IP $FLOATING_IP"
  hcloud floating-ip assign "$FLOATING_IP" "$SERVER_NAME"
  FIP=$(hcloud floating-ip describe "$FLOATING_IP" -o json | jq -r .ip)
  rssh "cat > /etc/netplan/60-floating-ip.yaml <<EOF
network:
  version: 2
  ethernets:
    eth0:
      addresses: [$FIP/32]
EOF
chmod 600 /etc/netplan/60-floating-ip.yaml && netplan apply"
fi

# ---------------------------------------------------------------- 3. bootstrap
log "3. Bootstrap k3s, secrets, Flux (deploy/hetzner/bootstrap.sh)"
if done_step bootstrap; then
  echo "already done"
else
  scp -q "$REPO_DIR/deploy/hetzner/bootstrap.sh" "root@$IP:/root/bootstrap.sh"
  if [[ -n "${FLUX_DEPLOY_KEY_FILE:-}" ]]; then
    rssh 'install -d -m 700 /root/.ssh'
    scp -q "$FLUX_DEPLOY_KEY_FILE" "root@$IP:/root/.ssh/flux-deploy-key"
    rssh 'chmod 600 /root/.ssh/flux-deploy-key'
  fi
  # Values travel in the SSH session only; printf %q keeps them shell safe.
  # Passing the crypt keys makes the new cluster write and read the same backups.
  envs=""
  for v in $BOOT_VARS; do
    envs+="$v=$(printf %q "${!v}") "
  done
  # bootstrap.sh seeds tester accounts on the empty database; they are
  # discarded below when the database is replaced by the dump.
  rssh "$envs bash /root/bootstrap.sh < /dev/null" > bootstrap-restore.log 2>&1 \
    || { tail -n 30 bootstrap-restore.log; die "bootstrap stopped, see bootstrap-restore.log (if it printed a deploy key: add it in GitHub with write access, then rerun)"; }
  mark_step bootstrap
fi

# ---------------------------------------------------------------- 4. freeze
log "4. Suspend Flux and stop the platform (no writes during restore)"
rssh "flux suspend kustomization mai && kubectl -n $NS scale deploy/mai --replicas=0 \
  && kubectl -n $NS wait --for=delete pod -l app=mai --timeout=120s || true; \
  kubectl -n $NS rollout status statefulset/postgres --timeout=300s"
# While mai is down, inbound SMTP is refused at TCP level and senders retry
# (RFC 5321 section 4.5.4.1); keep this window short.

# Every job script starts by giving rclone the crypt keys (obscured form).
CRYPT_PRELUDE='for r in DUMPS BLOBS; do
  export "RCLONE_CONFIG_${r}_PASSWORD=$(rclone obscure "$BACKUP_CRYPT_PASSWORD")" "RCLONE_CONFIG_${r}_PASSWORD2=$(rclone obscure "$BACKUP_CRYPT_SALT")"
done
'
job() { # job NAME RUN_AS_UID PVC MOUNT SCRIPT
  rssh "kubectl -n $NS delete job $1 --ignore-not-found >/dev/null; kubectl -n $NS apply -f -" <<EOF
apiVersion: batch/v1
kind: Job
metadata: { name: $1, labels: { dr.proposal: restore } }
spec:
  backoffLimit: 1
  template:
    metadata: { labels: { app: backup } }
    spec:
      restartPolicy: Never
      securityContext: { runAsUser: $2, runAsGroup: $2, fsGroup: $2, runAsNonRoot: true }
      containers:
        - name: run
          image: rclone/rclone:1.75.2
          command: ["/bin/sh", "-ec", $(jq -Rs . <<<"$CRYPT_PRELUDE$5")]
          env:
            - { name: HOME, value: /tmp }
            - { name: DUMP, value: "${DUMP:-}" }
            - name: BACKUP_S3_BUCKET
              valueFrom: { secretKeyRef: { name: mai-backup, key: BACKUP_S3_BUCKET } }
            - { name: RCLONE_CONFIG_STORE_TYPE, value: s3 }
            - { name: RCLONE_CONFIG_STORE_PROVIDER, value: Hetzner }
            - { name: RCLONE_CONFIG_STORE_NO_CHECK_BUCKET, value: "true" }
            - name: RCLONE_CONFIG_STORE_ENDPOINT
              valueFrom: { secretKeyRef: { name: mai-backup, key: BACKUP_S3_ENDPOINT } }
            - name: RCLONE_CONFIG_STORE_ACCESS_KEY_ID
              valueFrom: { secretKeyRef: { name: mai-backup, key: BACKUP_S3_ACCESS_KEY } }
            - name: RCLONE_CONFIG_STORE_SECRET_ACCESS_KEY
              valueFrom: { secretKeyRef: { name: mai-backup, key: BACKUP_S3_SECRET_KEY } }
            - { name: RCLONE_CONFIG_DUMPS_TYPE, value: crypt }
            - { name: RCLONE_CONFIG_DUMPS_REMOTE, value: "store:\$(BACKUP_S3_BUCKET)/crypt/dumps" }
            - { name: RCLONE_CONFIG_DUMPS_DIRECTORY_NAME_ENCRYPTION, value: "false" }
            - { name: RCLONE_CONFIG_BLOBS_TYPE, value: crypt }
            - { name: RCLONE_CONFIG_BLOBS_REMOTE, value: "store:\$(BACKUP_S3_BUCKET)/crypt/blobs" }
            - name: BACKUP_CRYPT_PASSWORD
              valueFrom: { secretKeyRef: { name: mai-backup, key: BACKUP_CRYPT_PASSWORD } }
            - name: BACKUP_CRYPT_SALT
              valueFrom: { secretKeyRef: { name: mai-backup, key: BACKUP_CRYPT_SALT } }
          volumeMounts: [ { name: target, mountPath: $4 } ]
      volumes:
        - { name: target, persistentVolumeClaim: { claimName: $3 } }
EOF
  rssh "kubectl -n $NS wait --for=condition=complete job/$1 --timeout=6h" \
    || { rssh "kubectl -n $NS logs job/$1 --tail=50" || true; die "job $1 failed"; }
  rssh "kubectl -n $NS logs job/$1 --tail=20"
}

# ---------------------------------------------------------------- 5. database
log "5. Restore Postgres from the dump"
if done_step database; then
  echo "already done"
else
  # Download straight onto the Postgres volume (single node: RWO is shared on the node).
  # Download and decrypt straight onto the Postgres volume (single node: RWO is shared on the node).
  job restore-fetch-dump 70 data-postgres-0 /pgdata '
    name="${DUMP:-$(rclone lsf -R --files-only --include "mai-*.dump" dumps: | sort | tail -n 1)}"
    [ -n "$name" ] || { echo "no dump found"; exit 1; }
    rclone copyto "dumps:$name" /pgdata/restore.dump
    rclone copyto "dumps:$name.counts" /pgdata/restore.counts
    echo "fetched $name, $(stat -c %s /pgdata/restore.dump) bytes"'
  rssh "kubectl -n $NS exec -i postgres-0 -- sh -ec '
    cd /var/lib/postgresql/data
    pg_restore --list restore.dump >/dev/null
    psql -v ON_ERROR_STOP=1 -U postgres -d postgres -c \"drop database if exists mai with (force)\"
    psql -v ON_ERROR_STOP=1 -U postgres -d postgres -c \"create database mai owner $DB_OWNER\"
    pg_restore -U postgres -d mai --exit-on-error restore.dump
    /docker-entrypoint-initdb.d/10-mai.sh
    rm -f restore.dump'"
  # Row counts of every table must equal the counts the backup job recorded
  # from the same snapshot as the dump.
  rssh "kubectl -n $NS exec postgres-0 -- cat /var/lib/postgresql/data/restore.counts" > restore-backup.counts
  rssh "kubectl -n $NS exec -i postgres-0 -- psql -X -q -At -v ON_ERROR_STOP=1 -U postgres -d mai" > restore-restored.counts <<'SQL'
select format('select %L, count(*) from %I.%I', tablename, schemaname, tablename) from pg_tables where schemaname = 'public' order by tablename \gexec
SQL
  if ! cmp -s restore-backup.counts restore-restored.counts; then
    diff restore-backup.counts restore-restored.counts || true
    die "restored row counts differ from the backup (files restore-*.counts)"
  fi
  echo "row counts match the backup: $(tr '\n' ' ' < restore-restored.counts)"
  rssh "kubectl -n $NS exec postgres-0 -- rm -f /var/lib/postgresql/data/restore.counts"
  mark_step database
fi

# ---------------------------------------------------------------- 6. blobs
if [[ "${SKIP_BLOBS:-}" == 1 ]]; then
  log "6. Blobs skipped (SKIP_BLOBS=1)"
elif done_step blobs; then
  log "6. Blobs already restored"
else
  log "6. Restore blobs (rclone copy, safe to rerun)"
  job restore-blobs 65532 mai-blobs /blobs '
    rclone copy --transfers 16 blobs: /blobs
    echo "blob files: $(find /blobs -type f | wc -l)"'
  mark_step blobs
fi

# ---------------------------------------------------------------- 7. resume
log "7. Resume Flux and the platform"
rssh "flux resume kustomization mai && flux reconcile kustomization mai --with-source && kubectl -n $NS rollout status deploy/mai --timeout=300s"

# ---------------------------------------------------------------- 8. verify
log "8. Verification"
psql_q() { rssh "kubectl -n $NS exec postgres-0 -- psql -U postgres -d mai -AtXc $(printf %q "$1")"; }
echo "row counts:"
for t in organizations inboxes messages attachments events webhook_deliveries audit_log schema_migrations; do
  printf '  %-20s %s\n' "$t" "$(psql_q "select count(*) from $t")"
done
echo "newest message: $(psql_q 'select max(created_at) from messages')"

ready=$(rssh "curl -s -o /dev/null -w '%{http_code}' http://\$(kubectl -n $NS get svc mai-http -o jsonpath='{.spec.clusterIP}'):8080/readyz")
[[ "$ready" == 204 ]] || die "/readyz returned $ready"
echo "/readyz in cluster: 204"
if [[ -n "${API_HOST:-}" ]]; then
  code=$(curl -s -o /dev/null -w '%{http_code}' "https://$API_HOST/readyz" || true)
  echo "/readyz via https://$API_HOST: $code (certificate issuance can take a minute; DNS must point at $IP)"
fi

if [[ "${SKIP_BLOBS:-}" != 1 ]]; then
  key=$(psql_q "select raw_key from messages where raw_key is not null order by created_at desc limit 1")
  if [[ -n "$key" ]]; then
    job restore-check-blob 65532 mai-blobs /blobs "test -s /blobs/$key && echo 'newest raw message present: $key'"
  else
    echo "no messages with raw_key, blob check skipped"
  fi
fi

if [[ -n "${SMOKE_API_KEY:-}" && -n "${API_HOST:-}" ]]; then
  API=https://$API_HOST
  curl -fsS -X POST "$API/v0/inboxes/user.tester@agents.localhost/messages/send" -H "Authorization: Bearer $SMOKE_API_KEY" \
    -d '{"to":["admin.tester@agents.localhost"],"subject":"dr smoke","text":"restore check"}' >/dev/null
  sleep 5
  curl -fsS "$API/v0/inboxes/admin.tester@agents.localhost/messages" -H "Authorization: Bearer $SMOKE_API_KEY" | grep -q 'dr smoke' \
    && echo "send/receive smoke test: OK" || die "smoke test message not received"
fi

log "9. Protect the new server (daily Hetzner backups, delete and rebuild protection)"
hcloud server enable-backup "$SERVER_NAME" || echo "WARN: enable-backup failed, enable it in the console"
hcloud server enable-protection "$SERVER_NAME" delete rebuild || echo "WARN: enable-protection failed"

rssh "kubectl -n $NS delete job -l dr.proposal=restore --ignore-not-found >/dev/null"
cat <<EOF

Restore finished on $SERVER_NAME ($IP). Still manual:
  * If the IP changed: the sslip.io names in deploy/k8s/staging/kustomization.yaml embed the IP; commit new names
    (or move the Primary/Floating IP) and later switch DNS records for the brand domain.
  * Tester API keys printed by bootstrap.sh in bootstrap-restore.log are void (that database was replaced); delete the log.
  * Customer API keys, webhooks and DKIM keys came back with the dump. Mail accepted after the dump time is lost.
  * Trigger a backup now: ssh root@$IP kubectl -n $NS create job --from=cronjob/backup backup-post-restore-\$(date +%s)
EOF
