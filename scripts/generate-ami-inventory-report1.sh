#!/usr/bin/env bash
# scripts/generate-ami-inventory-report.sh
#
# Generates a CAPA AMIs Inventory report named:
#   "CAPA AMIs Inventory - <ISO date>.md"
#
# Prerequisites:
#   - jq  (brew install jq)
#   - AWS credentials configured (for clusterawsadm ami list)
#   - bin/clusterawsadm already built, or Go available to build it
#   - Optional: GITHUB_TOKEN set + bin/release-tool built to fetch fresh K8s
#     versions from GitHub; otherwise falls back to AMIBuildConfig.json
#
# Usage:
#   ./scripts/generate-ami-inventory-report.sh
#
# Override defaults with environment variables:
#   OWNER_ID=027487054958  K8S_CONFIG_PATH=...  OUTPUT_DIR=/tmp  \
#     ./scripts/generate-ami-inventory-report.sh

set -euo pipefail

# ── Configuration ──────────────────────────────────────────────────────────

OWNER_ID="${OWNER_ID:-027487054958}"
K8S_CONFIG_PATH="${K8S_CONFIG_PATH:-hack/tools/release-tools/ami/k8srelease/data/AMIBuildConfig.json}"
OUTPUT_DIR="${OUTPUT_DIR:-hack/tools/release-tools/ami/report}"

# Default scope for the Missing AMI comparison (space-separated).
DEFAULT_OS_LIST="${DEFAULT_OS_LIST:-ubuntu-24.04 ubuntu-22.04}"
DEFAULT_REGION_LIST="${DEFAULT_REGION_LIST:-ap-southeast-2}"

# ── Boilerplate ────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

REPORT_DATE="$(date -u +%Y-%m-%d)"
REPORT_FILE="${OUTPUT_DIR}/CAPA AMIs Inventory - ${REPORT_DATE}.md"

die()  { echo "ERROR: $*" >&2; exit 1; }
info() { echo "==> $*" >&2; }

command -v jq &>/dev/null || die "'jq' is required but not found. Install with: brew install jq"

# ── Step 1: K8s versions ───────────────────────────────────────────────────
# Prefer a live GitHub query when GITHUB_TOKEN + release-tool are available;
# fall back to the committed AMIBuildConfig.json.

info "Resolving Kubernetes versions..."
if [ -n "${GITHUB_TOKEN:-}" ] && [ -x "bin/release-tool" ]; then
  info "  source: GitHub API (release-tool)"
  K8S_JSON="$(./bin/release-tool ami detect-k8s-release \
    --token "$GITHUB_TOKEN" --output json)"
elif [ -f "${K8S_CONFIG_PATH}" ]; then
  info "  source: ${K8S_CONFIG_PATH}"
  K8S_JSON="$(cat "${K8S_CONFIG_PATH}")"
else
  die "No K8s version source found. Either set GITHUB_TOKEN and build bin/release-tool, or ensure ${K8S_CONFIG_PATH} exists."
fi

# Validate that we have at least one version entry.
VERSION_COUNT="$(echo "${K8S_JSON}" | jq '.versions | length')"
[ "${VERSION_COUNT}" -gt 0 ] || die "K8s version list is empty."

# ── Step 2: Build clusterawsadm if needed ─────────────────────────────────

if [ ! -x "bin/clusterawsadm" ]; then
  info "bin/clusterawsadm not found — building (this may take a minute)..."
  go build -o bin/clusterawsadm ./cmd/clusterawsadm
fi

# ── Step 3: AMI list ───────────────────────────────────────────────────────

info "Fetching AMI list from AWS account ${OWNER_ID}..."
AMI_JSON="$(./bin/clusterawsadm ami list --owner-id "${OWNER_ID}" -o json 2>/dev/null \
  || echo '{"apiVersion":"","kind":"","items":[]}')"

AMI_COUNT="$(echo "${AMI_JSON}" | jq '.items | length')"
info "  found ${AMI_COUNT} AMI(s)."

# ── Helper: render Markdown tables ─────────────────────────────────────────

k8s_table() {
  echo "| Minor Version | Patch Versions |"
  echo "| --- | --- |"
  echo "${K8S_JSON}" | jq -r '
    .versions[] |
    "| `" + .minor + "` | " + (.patches | map("`" + . + "`") | join(", ")) + " |"
  '
}

ami_table() {
  if [ "${AMI_COUNT}" -eq 0 ]; then
    echo "_No AMIs found._"
    return
  fi
  echo "| AMI Name | Kubernetes Version | OS | Region | AMI ID | Created |"
  echo "| --- | --- | --- | --- | --- | --- |"
  echo "${AMI_JSON}" | jq -r '
    .items[]? |
    "| `" + (.metadata.name            // "-") + "`" +
    " | `" + (.spec.kubernetesVersion  // "-") + "`" +
    " | "  + (.spec.os                 // "-") +
    " | "  + (.spec.region             // "-") +
    " | `" + (.spec.imageID            // "-") + "`" +
    " | "  + (
        if .metadata.creationTimestamp != null and .metadata.creationTimestamp != "0001-01-01T00:00:00Z"
        then .metadata.creationTimestamp
        else "-"
        end
      ) + " |"
  '
}

missing_ami_table() {
  # Build a normalised lookup set from published AMIs.
  # Normalise to "v<version>/<os>/<region>" for consistent comparison.
  local published
  published="$(echo "${AMI_JSON}" | jq -r '
    .items[]? |
    ( if (.spec.kubernetesVersion | startswith("v")) then "" else "v" end )
    + .spec.kubernetesVersion + "/" + .spec.os + "/" + .spec.region
  ')"

  # Collect all missing combinations.
  local rows=()
  while IFS= read -r raw_version; do
    local version="v${raw_version}"   # normalise to v-prefixed
    for os in ${DEFAULT_OS_LIST}; do
      for region in ${DEFAULT_REGION_LIST}; do
        local key="${version}/${os}/${region}"
        if ! grep -qF "${key}" <<< "${published}"; then
          rows+=("| \`${version}\` | ${os} | ${region} |")
        fi
      done
    done
  done < <(echo "${K8S_JSON}" | jq -r '.versions[].patches[]')

  if [ "${#rows[@]}" -eq 0 ]; then
    echo "No AMI missing for the configured Kubernetes versions, OS, and regions."
  else
    echo "| Kubernetes Version | OS | Region |"
    echo "| --- | --- | --- |"
    printf '%s\n' "${rows[@]}"
  fi
}

# ── Step 4: Render default-OS / default-region bullet lists ───────────────

os_bullets() {
  for os in ${DEFAULT_OS_LIST}; do echo "- ${os}"; done
}

region_bullets() {
  for region in ${DEFAULT_REGION_LIST}; do echo "- ${region}"; done
}

# ── Step 5: Write report ───────────────────────────────────────────────────

mkdir -p "${OUTPUT_DIR}"
info "Writing report → ${REPORT_FILE}"

cat > "${REPORT_FILE}" <<EOF
## Kubernetes Release

This section lists the Kubernetes versions tracked by this repository following the [CAPA AMI publication policy](https://cluster-api-aws.sigs.k8s.io/topics/images/built-amis#ami-publication-policy): the latest three supported minor releases and all their stable patch versions as detected by \`release-tool ami detect-k8s-release\`.

$(k8s_table)

## CAPA AMI

The table below lists all AMIs currently published in AWS account \`${OWNER_ID}\`, as returned by \`clusterawsadm ami list --owner-id ${OWNER_ID}\`.

$(ami_table)

## Missing AMI

### Default OS

List of OS for which AMIs should be published (default):

$(os_bullets)

### Default Region

List of regions for which AMIs should be published (default):

$(region_bullets)

### List of Missing AMIs

$(missing_ami_table)
EOF

echo "Report written: ${REPORT_FILE}"
