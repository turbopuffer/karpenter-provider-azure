#!/usr/bin/env bash
# Mirror the controller images for one tag from turbopuffer ACR to GAR, where
# the control plane's /docker-images endpoint serves BYOC customers from.
# Multi-arch manifests are copied as-is so the digests pinned in the kits
# stay valid.
set -euo pipefail

TAG="${1:-$(git describe --tags)}"
ACR="${ACR:-turbopuffer.azurecr.io/turbopuffer/karpenter-azure}"
GAR="${GAR:-us-central1-docker.pkg.dev/turbopuffer-onprem/turbopuffer}"
VARIANTS="${VARIANTS:-controller controller-fips}"

# skopeo reads the docker credential store both CLIs populate.
az acr login -n "${ACR%%.*}" >/dev/null
gcloud auth configure-docker "${GAR%%/*}" --quiet >/dev/null

for v in $VARIANTS; do
  src="docker://$ACR/$v:$TAG"
  dst="docker://$GAR/karpenter-azure-$v:$TAG"
  skopeo copy --all --preserve-digests "$src" "$dst"
  echo "$dst@$(skopeo inspect "$dst" | jq -r .Digest)"
done
