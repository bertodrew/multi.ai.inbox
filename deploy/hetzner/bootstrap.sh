#!/usr/bin/env bash
# One time bootstrap of the Hetzner CX33 (reinstalled with Ubuntu 24.04):
# k3s, Flux (GitOps) and the secrets that must never live in git.
# Run as root on the server:  curl -fsSLO <raw url>/deploy/hetzner/bootstrap.sh && bash bootstrap.sh
# Non interactive (an agent over SSH): export the variables below first; the
# script then never prompts and fails fast if a required one is missing.
#
# You need:
#   GITHUB_TOKEN  fine grained PAT on bertodrew/multi.ai.inbox: Contents read/write, Administration read/write
#                 (Flux commits its own manifests and a deploy key once)
#   GHCR_TOKEN    classic PAT with read:packages, so the cluster can pull the private image
#   Brevo SMTP login and key (Brevo > SMTP & API > SMTP)
#   Hetzner Object Storage bucket + S3 keys for backups (optional, backups fail until set)
set -euo pipefail

REPO_OWNER=bertodrew
REPO_NAME=multi.ai.inbox
BRANCH=master
FLUX_PATH=deploy/flux/clusters/staging
K3S_VERSION=v1.35.8+k3s1
FLUX_VERSION=2.9.6

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

echo "==> System updates and unattended security upgrades"
apt-get update -y
apt-get install -y unattended-upgrades curl jq
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

echo "==> Secrets (stored only in the cluster, encrypted at rest by k3s)"
ask GITHUB_TOKEN "GitHub PAT for Flux" secret
ask GHCR_TOKEN "GitHub PAT with read:packages for GHCR" secret
ask BREVO_LOGIN "Brevo SMTP login (email)"
ask BREVO_KEY "Brevo SMTP key" secret
ask BACKUP_S3_ENDPOINT "Object Storage endpoint (e.g. fsn1.your-objectstorage.com, empty to skip)" optional
if [[ -n "$BACKUP_S3_ENDPOINT" ]]; then
  ask BACKUP_S3_BUCKET "Bucket name"
  ask BACKUP_S3_ACCESS_KEY "S3 access key"
  ask BACKUP_S3_SECRET_KEY "S3 secret key" secret
fi

kubectl create namespace mai --dry-run=client -o yaml | kubectl apply -f -
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
kubectl -n mai create secret generic mai-backup \
  --from-literal=BACKUP_S3_ENDPOINT="${BACKUP_S3_ENDPOINT:-unset}" \
  --from-literal=BACKUP_S3_BUCKET="${BACKUP_S3_BUCKET:-unset}" \
  --from-literal=BACKUP_S3_ACCESS_KEY="${BACKUP_S3_ACCESS_KEY:-unset}" \
  --from-literal=BACKUP_S3_SECRET_KEY="${BACKUP_S3_SECRET_KEY:-unset}" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n mai create secret docker-registry ghcr-pull \
  --docker-server=ghcr.io --docker-username="$REPO_OWNER" --docker-password="$GHCR_TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "==> Flux bootstrap (GitOps from ${REPO_OWNER}/${REPO_NAME}@${BRANCH}:${FLUX_PATH})"
GITHUB_TOKEN="$GITHUB_TOKEN" flux bootstrap github \
  --owner="$REPO_OWNER" --repository="$REPO_NAME" --branch="$BRANCH" \
  --path="$FLUX_PATH" --personal --token-auth=false

echo "==> Waiting for the platform"
flux reconcile kustomization mai --with-source || true
kubectl -n mai rollout status deploy/mai --timeout=300s

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
