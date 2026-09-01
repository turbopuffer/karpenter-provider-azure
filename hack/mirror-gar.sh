#!/usr/bin/env bash
# Mirror the controller images for one tag from turbopuffer ACR to GAR, where
# the control plane's /docker-images endpoint serves BYOC customers from.
# Multi-arch manifests are copied as-is so the digests pinned in the kits
# stay valid.
#
# Usage:
#   hack/mirror-gar.sh [--skip-registry-auth] [TAG]
#
# By default logs into ACR via `az acr login`. Pass --skip-registry-auth when
# ACR creds are already in the docker config (CI). Always runs
# `gcloud auth configure-docker` for GAR.
set -euo pipefail

SKIP_REGISTRY_AUTH=0
if [[ "${1:-}" == "--skip-registry-auth" ]]; then
  SKIP_REGISTRY_AUTH=1
  shift
fi

TAG="${1:-$(git describe --tags)}"
ACR="${ACR:-turbopuffer.azurecr.io/turbopuffer/karpenter-azure}"
GAR="${GAR:-us-central1-docker.pkg.dev/turbopuffer-onprem/turbopuffer}"
VARIANTS="${VARIANTS:-controller controller-fips}"

# skopeo reads the docker credential store. ACR login is optional for CI
# (docker/login-action already populated creds); gcloud configure-docker is
# cheap and needs a gcloud identity either way.
if [[ "${SKIP_REGISTRY_AUTH}" -eq 0 ]]; then
  az acr login -n "${ACR%%.*}" >/dev/null
fi
gcloud auth configure-docker "${GAR%%/*}" --quiet >/dev/null

for v in $VARIANTS; do
  src="docker://$ACR/$v:$TAG"
  dst="docker://$GAR/karpenter-azure-$v:$TAG"
  skopeo copy --all --preserve-digests "$src" "$dst"
  echo "$dst@sha256:$(skopeo inspect --raw "$dst" | shasum -a 256 | cut -d' ' -f1)"
done
