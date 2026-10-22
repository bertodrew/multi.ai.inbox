#!/usr/bin/env bash
# One time bootstrap of the Hetzner CX33 (reinstalled with Ubuntu 24.04):
# k3s, Flux (GitOps) and the secrets that must never live in git.
# Run as root on the server:  curl -fsSLO <raw url>/deploy/hetzner/bootstrap.sh && bash bootstrap.sh
# Non interactive (an agent over SSH): export the variables below first; the
# script then never prompts and fails fast if a required one is missing.
# Rerunnable: existing secret values (database passwords, backup and crypt
# keys) are kept unless the variable is passed again.
#
# You need:
#   Flux access to GitHub, one of (see AGENT_RUNBOOK.md section 0):
#     * default, no GitHub token: an SSH deploy key. The first run generates
#       /root/.ssh/flux-deploy-key (or uses FLUX_DEPLOY_KEY_FILE), prints the
#       public key and stops; add it in GitHub > repo > Settings > Deploy keys
#       with "Allow write access" (bootstrap pushes its manifests), rerun, then
#       re-add the same key without write access.
#     * GITHUB_TOKEN set: fine grained PAT on bertodrew/multi.ai.inbox with
#       Contents read/write, Administration read/write, Metadata read. Flux
#       creates the deploy key with it; per the Flux docs that key stops working
#       when the PAT is revoked or expires, so prefer the deploy key path.
#   GHCR_TOKEN    classic PAT with read:packages only, so the cluster can pull the private image
#   Brevo SMTP login and key (Brevo > SMTP & API > SMTP)
#   Hetzner Object Storage bucket + S3 keys for backups (optional, backups fail until set)
#   BACKUP_CRYPT_PASSWORD, BACKUP_CRYPT_SALT (optional): rclone crypt keys for
#     the backups. Generated on the first run when absent; pass the stored ones
#     when rebuilding a server so it can read the existing backups.
set -euo pipefail

REPO_OWNER=bertodrew
REPO_NAME=multi.ai.inbox
BRANCH=master
FLUX_PATH=deploy/flux/clusters/staging
K3S_VERSION=v1.35.8+k3s1
FLUX_VERSION=2.9.6
FLUX_DEPLOY_KEY_FILE=${FLUX_DEPLOY_KEY_FILE:-/root/.ssh/flux-deploy-key}

[[ $EUID -eq 0 ]] || { echo "run as root"; exit 1; }

ask() { # ask VAR "prompt" [secret|optional]
  local __v
  if [[ -n "${!1:-}" ]]; then return; fi
  if [[ ! -t 0 ]]; then # non interactive (automation, agents): values must come from the environment
    if [[ "${3:-}" == optional ]]; then printf -v "$1" '%s' ""; return; fi
    echo "missing required environment variable $1 ($2)" >&2; exit 1
  fi
  if [[ "${3:-}" == secret ]]; then read -rsp "$2: " __v; echo; else read -rp "$2: " __v; fi
  printf -v "$1" '%s' "$__v"
}

secret_value() { # secret_value SECRET KEY: current value in namespace mai, empty if absent
  kubectl -n mai get secret "$1" -o "jsonpath={.data.$2}" 2>/dev/null | base64 -d 2>/dev/null || true
}

keep() { # keep VAR SECRET KEY: when VAR is not passed, reuse the value already in the cluster
  local __v
  if [[ -n "${!1:-}" ]]; then return; fi
  __v=$(secret_value "$2" "$3")
  [[ "$__v" == unset ]] && __v=""
  printf -v "$1" '%s' "$__v"
}

echo "==> System updates and unattended security upgrades"
apt-get update -y
apt-get install -y unattended-upgrades curl jq openssh-client
dpkg-reconfigure -f noninteractive unattended-upgrades

echo "==> k3s ${K3S_VERSION} (Traefik disabled: Caddy terminates TLS; ServiceLB kept for ports 25/80/443)"
if ! command -v k3s >/dev/null; then
  curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="$K3S_VERSION" sh -s - server \
    --disable traefik \
    --write-kubeconfig-mode 600 \
    --secrets-encryption
fi
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
until kubectl get nodes >/dev/null 2>&1; do sleep 2; done
kubectl wait --for=condition=Ready node --all --timeout=180s

echo "==> Flux CLI ${FLUX_VERSION}"
if ! command -v flux >/dev/null; then
  curl -sfL https://fluxcd.io/install.sh | FLUX_VERSION="$FLUX_VERSION" bash
fi

echo "==> Flux access to GitHub"
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  FLUX_AUTH=github-pat
  echo "GITHUB_TOKEN set: flux bootstrap github creates the deploy key"
else
  FLUX_AUTH=deploy-key
  if [[ ! -f "$FLUX_DEPLOY_KEY_FILE" ]]; then
    install -d -m 700 "$(dirname "$FLUX_DEPLOY_KEY_FILE")"
    ssh-keygen -q -t ed25519 -N "" -C "flux-staging@$(hostname)" -f "$FLUX_DEPLOY_KEY_FILE"
  fi
  chmod 600 "$FLUX_DEPLOY_KEY_FILE"
  # GitHub answers "successfully authenticated" (exit status 1) once the deploy key is registered.
  if ! ssh -T -i "$FLUX_DEPLOY_KEY_FILE" -o IdentitiesOnly=yes -o BatchMode=yes \
       -o StrictHostKeyChecking=accept-new git@github.com 2>&1 | grep -q "successfully authenticated"; then
    cat <<EOF

ACTION NEEDED: add this public key in GitHub > ${REPO_OWNER}/${REPO_NAME} > Settings > Deploy keys,
title "flux-staging", with "Allow write access" ticked (bootstrap pushes the flux-system manifests once):

$(cat "$FLUX_DEPLOY_KEY_FILE.pub")

Then run this script again (it continues from here). No GitHub token is needed.
EOF
    exit 3
  fi
fi

echo "==> Secrets (stored only in the cluster, encrypted at rest by k3s)"
kubectl create namespace mai --dry-run=client -o yaml | kubectl apply -f -
ask GHCR_TOKEN "GitHub PAT with read:packages for GHCR" secret
ask BREVO_LOGIN "Brevo SMTP login (email)"
ask BREVO_KEY "Brevo SMTP key" secret
keep BACKUP_S3_ENDPOINT mai-backup BACKUP_S3_ENDPOINT
keep BACKUP_S3_BUCKET mai-backup BACKUP_S3_BUCKET
keep BACKUP_S3_ACCESS_KEY mai-backup BACKUP_S3_ACCESS_KEY
keep BACKUP_S3_SECRET_KEY mai-backup BACKUP_S3_SECRET_KEY
ask BACKUP_S3_ENDPOINT "Object Storage endpoint (e.g. https://hel1.your-objectstorage.com, empty to skip)" optional
if [[ -n "$BACKUP_S3_ENDPOINT" ]]; then
  ask BACKUP_S3_BUCKET "Bucket name"
  ask BACKUP_S3_ACCESS_KEY "S3 access key (write only key, see AGENT_RUNBOOK.md)"
  ask BACKUP_S3_SECRET_KEY "S3 secret key" secret
fi
for v in BACKUP_CRYPT_PASSWORD BACKUP_CRYPT_SALT BACKUP_PG_USER BACKUP_PG_PASSWORD HC_BACKUP_URL HC_RESTORE_DRILL_URL; do
  keep "$v" mai-backup "$v"
done

CRYPT_GENERATED=""
if [[ -z "$BACKUP_CRYPT_PASSWORD" || -z "$BACKUP_CRYPT_SALT" ]]; then
  [[ -z "$BACKUP_CRYPT_PASSWORD" && -z "$BACKUP_CRYPT_SALT" ]] \
    || { echo "set both BACKUP_CRYPT_PASSWORD and BACKUP_CRYPT_SALT, or neither" >&2; exit 1; }
  BACKUP_CRYPT_PASSWORD=$(openssl rand -hex 32)
  BACKUP_CRYPT_SALT=$(openssl rand -hex 32)
  CRYPT_GENERATED=1
fi

POSTGRES_PASSWORD=$(openssl rand -hex 24)
MAI_APP_PASSWORD=$(openssl rand -hex 24)
if ! kubectl -n mai get secret mai-secrets >/dev/null 2>&1; then
  kubectl -n mai create secret generic mai-secrets \
    --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
    --from-literal=MAI_APP_PASSWORD="$MAI_APP_PASSWORD" \
    --from-literal=MAI_SMTP_RELAY="smtp://$(jq -rn --arg v "$BREVO_LOGIN" '$v|@uri'):$(jq -rn --arg v "$BREVO_KEY" '$v|@uri')@smtp-relay.brevo.com:587"
else
  echo "mai-secrets exists, keeping the database passwords"
fi

write_backup_secret() {
  local args=(
    --from-literal=BACKUP_S3_ENDPOINT="${BACKUP_S3_ENDPOINT:-unset}"
    --from-literal=BACKUP_S3_BUCKET="${BACKUP_S3_BUCKET:-unset}"
    --from-literal=BACKUP_S3_ACCESS_KEY="${BACKUP_S3_ACCESS_KEY:-unset}"
    --from-literal=BACKUP_S3_SECRET_KEY="${BACKUP_S3_SECRET_KEY:-unset}"
    --from-literal=BACKUP_CRYPT_PASSWORD="$BACKUP_CRYPT_PASSWORD"
    --from-literal=BACKUP_CRYPT_SALT="$BACKUP_CRYPT_SALT"
  )
  local v
  for v in BACKUP_PG_USER BACKUP_PG_PASSWORD HC_BACKUP_URL HC_RESTORE_DRILL_URL; do
    [[ -n "${!v:-}" ]] && args+=(--from-literal="$v=${!v}")
  done
  kubectl -n mai create secret generic mai-backup "${args[@]}" --dry-run=client -o yaml | kubectl apply -f -
}
write_backup_secret

if [[ -n "$CRYPT_GENERATED" ]]; then
  cat <<EOF

################################################################################
#  BACKUP ENCRYPTION KEYS: SHOWN ONCE. STORE THEM OFFLINE NOW.
#
#  BACKUP_CRYPT_PASSWORD=$BACKUP_CRYPT_PASSWORD
#  BACKUP_CRYPT_SALT=$BACKUP_CRYPT_SALT
#
#  Every backup in Object Storage is encrypted with these two values (rclone
#  crypt). Without them the backups CANNOT be restored, by anyone, ever.
#  The founder keeps two offline copies (password manager entry plus paper in a
#  safe). Never commit them, never paste them in chat, issues or tickets.
#  When rebuilding a server, pass both variables to this script again.
#  An agent running this script hands this block to the founder privately and
#  then deletes it from its logs.
################################################################################

EOF
fi

kubectl -n mai create secret docker-registry ghcr-pull \
  --docker-server=ghcr.io --docker-username="$REPO_OWNER" --docker-password="$GHCR_TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "==> Flux bootstrap (GitOps from ${REPO_OWNER}/${REPO_NAME}@${BRANCH}:${FLUX_PATH})"
if [[ "$FLUX_AUTH" == github-pat ]]; then
  GITHUB_TOKEN="$GITHUB_TOKEN" flux bootstrap github \
    --owner="$REPO_OWNER" --repository="$REPO_NAME" --branch="$BRANCH" \
    --path="$FLUX_PATH" --personal --token-auth=false
else
  flux bootstrap git \
    --url="ssh://git@github.com/${REPO_OWNER}/${REPO_NAME}" --branch="$BRANCH" \
    --path="$FLUX_PATH" --private-key-file="$FLUX_DEPLOY_KEY_FILE" --silent
fi

echo "==> Waiting for the platform"
flux reconcile kustomization mai --with-source || true
kubectl -n mai rollout status statefulset/postgres --timeout=300s
kubectl -n mai rollout status deploy/mai --timeout=300s

echo "==> Read only Postgres role for the backup job (mai_backup)"
# pg_dump must see every tenant's rows through FORCE ROW LEVEL SECURITY, hence
# BYPASSRLS; pg_read_all_data gives SELECT everywhere and nothing else. Roles
# are cluster wide, so this survives a database restore. Idempotent.
if [[ -z "$BACKUP_PG_PASSWORD" ]]; then BACKUP_PG_PASSWORD=$(openssl rand -hex 24); fi
kubectl -n mai exec -i postgres-0 -- psql -q -v ON_ERROR_STOP=1 -U postgres -d mai <<SQL_BACKUP_ROLE
select 'create role mai_backup' where not exists (select from pg_roles where rolname = 'mai_backup') \gexec
alter role mai_backup with login nosuperuser nocreatedb nocreaterole noreplication bypassrls password '$BACKUP_PG_PASSWORD';
alter role mai_backup set default_transaction_read_only = on;
grant pg_read_all_data to mai_backup;
grant connect on database mai to mai_backup;
SQL_BACKUP_ROLE
BACKUP_PG_USER=mai_backup
write_backup_secret

echo "==> Staging seed (tester accounts; API keys are printed once, store them now)"
kubectl -n mai exec deploy/mai -- mai admin seed
echo "==> Outbound port 25 check"
kubectl -n mai exec deploy/mai -- mai admin check-port25

cat <<EOF

Done. Next steps:
  * Hetzner Cloud firewall: allow inbound 22 (your IP only), 25, 80, 443; nothing else.
  * Reverse DNS (PTR) of the server IP: mx.2-28-99-252.sslip.io (Hetzner console > server > Networking).
  * API: https://api.2-28-99-252.sslip.io/healthz
  * From now on, merging to ${BRANCH} deploys: CI builds the image and updates deploy/k8s/staging; Flux applies it.
EOF
if [[ "$FLUX_AUTH" == deploy-key ]]; then
  echo "  * GitHub deploy key: delete \"flux-staging\" and add the same public key again WITHOUT write access (Flux only reads)."
  echo "    The private key lives in secret flux-system/flux-system and in $FLUX_DEPLOY_KEY_FILE (root only, needed to rerun this script)."
else
  echo "  * GITHUB_TOKEN: keep it until you move to a deploy key (revoking it disables the deploy key Flux created with it)."
fi
if [[ -n "$CRYPT_GENERATED" ]]; then
  echo "  * BACKUP ENCRYPTION KEYS were generated: the block above must now be stored offline by the founder."
fi
echo "  * Run a first backup and the restore drill: see AGENT_RUNBOOK.md section 4b."
