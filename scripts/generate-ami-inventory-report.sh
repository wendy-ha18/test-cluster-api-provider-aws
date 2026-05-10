#!/usr/bin/env bash
# scripts/generate-ami-inventory-report.sh
#
# Populates the generated sections of docs/book/src/development/ami/ami-inventory.md
# with live data: K8s versions, published AMIs (requires AWS credentials),
# and default configuration values.
#
# Prerequisites:
#   - jq  (brew install jq)
#   - Optional: AWS credentials + bin/clusterawsadm for the AMI list section
#   - Optional: GITHUB_TOKEN + bin/release-tool for live K8s versions;
#               otherwise falls back to AMIBuildConfig.json
#
# Required environment variables (must match env: in build-and-publish-ami.yml
# and build-ami-varsfile.yml):
#   AWS_ACCOUNT_ID=027487054958
#   DEFAULT_REGIONS=ap-southeast-2
#
# Optional overrides:
#   DEFAULT_OS=ubuntu-24.04   K8S_CONFIG_PATH=...   TARGET_FILE=...

set -euo pipefail

# ── Configuration ──────────────────────────────────────────────────────────

# AWS_ACCOUNT_ID and DEFAULT_REGIONS must match the env: vars defined in
# build-and-publish-ami.yml and build-ami-varsfile.yml.
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-027487054958}"
DEFAULT_REGIONS="${DEFAULT_REGIONS:-ap-southeast-2 us-west-2}"

# DEFAULT_OS matches matrix.target in build-and-publish-ami.yml.
# Space-separated to support multiple OS values.
DEFAULT_OS="${DEFAULT_OS:-ubuntu-24.04 ubuntu-22.04}"

K8S_CONFIG_PATH="${K8S_CONFIG_PATH:-hack/tools/release-tools/ami/k8srelease/data/AMIBuildConfig.json}"
TARGET_FILE="${TARGET_FILE:-docs/book/src/development/ami/ami-inventory.md}"

# ── Boilerplate ────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

REPORT_DATE="$(date -u +%Y-%m-%d)"

die()  { echo "ERROR: $*" >&2; exit 1; }
info() { echo "==> $*" >&2; }

command -v jq &>/dev/null || die "'jq' is required but not found. Install with: brew install jq"
[ -f "${TARGET_FILE}" ] || die "Target file not found: ${TARGET_FILE}"

# ── Helpers ────────────────────────────────────────────────────────────────

# replace_after_marker <variable-name> <content>
# Finds <!-- $<name> --> and replaces the lines that follow it (up to the next
# ## or ### heading) with <content>. Idempotent — reruns overwrite previous output.
replace_after_marker() {
  local marker="<!-- \$$1 -->"
  local content="$2"
  local tmp
  tmp="$(mktemp)"
  printf '%s\n' "${content}" > "${tmp}"

  awk -v m="${marker}" -v f="${tmp}" '
    skip && /^#{2,3} / { skip=0 }
    skip               { next }
    index($0, m)       { print; while ((getline line < f) > 0) print line; skip=1; next }
    { print }
  ' "${TARGET_FILE}" > "${TARGET_FILE}.tmp"
  mv "${TARGET_FILE}.tmp" "${TARGET_FILE}"
  rm -f "${tmp}"
}

# ── Step 1: Update date in title ───────────────────────────────────────────

info "Setting report date to ${REPORT_DATE}..."
sed -i.bak "s/^# CAPA AMIs Inventory .*/# CAPA AMIs Inventory ${REPORT_DATE}/" "${TARGET_FILE}" \
  && rm -f "${TARGET_FILE}.bak"

# ── Step 2: K8s versions ───────────────────────────────────────────────────
# Prefer a live GitHub query when GITHUB_TOKEN + release-tool are available;
# fall back to the committed AMIBuildConfig.json.

info "Resolving Kubernetes versions..."
if [ -n "${GITHUB_TOKEN:-}" ] && [ -x "bin/release-tool" ]; then
  info "  source: GitHub API (release-tool)"
  K8S_JSON="$(./bin/release-tool ami detect-k8s-release --token "$GITHUB_TOKEN" --output json)"
elif [ -f "${K8S_CONFIG_PATH}" ]; then
  info "  source: ${K8S_CONFIG_PATH}"
  K8S_JSON="$(cat "${K8S_CONFIG_PATH}")"
else
  die "No K8s version source. Set GITHUB_TOKEN and build bin/release-tool, or ensure ${K8S_CONFIG_PATH} exists."
fi

VERSION_COUNT="$(echo "${K8S_JSON}" | jq '.versions | length')"
[ "${VERSION_COUNT}" -gt 0 ] || die "K8s version list is empty."

# ── Step 3: AMI list ───────────────────────────────────────────────────────
# Skipped when no AWS credentials are detected (e.g. CI without AWS access).

if [ -z "${AWS_ACCESS_KEY_ID:-}" ] && \
   [ -z "${AWS_WEB_IDENTITY_TOKEN_FILE:-}" ] && \
   [ -z "${AWS_ROLE_ARN:-}" ] && \
   [ -z "${AWS_PROFILE:-}" ] && \
   [ -z "${AWS_DEFAULT_PROFILE:-}" ] && \
   [ ! -f "${HOME}/.aws/credentials" ]; then
  info "No AWS credentials detected — skipping AMI list."
  AMI_JSON='{"apiVersion":"","kind":"","items":[]}'
else
  if [ ! -x "bin/clusterawsadm" ]; then
    info "bin/clusterawsadm not found — building (this may take a minute)..."
    go build -o bin/clusterawsadm ./cmd/clusterawsadm
  fi
  info "Fetching AMI list from AWS account ${AWS_ACCOUNT_ID}..."
  AMI_JSON="$(./bin/clusterawsadm ami list --owner-id "${AWS_ACCOUNT_ID}" -o json 2>/dev/null \
    || echo '{"apiVersion":"","kind":"","items":[]}')"
fi

AMI_COUNT="$(echo "${AMI_JSON}" | jq '.items | length')"
info "  found ${AMI_COUNT} AMI(s)."

# ── Step 4: Populate sections ──────────────────────────────────────────────

info "Populating default-os..."
os_content=""
for os in ${DEFAULT_OS}; do
  os_content="${os_content}- \`${os}\`
"
done
replace_after_marker "default_os" "${os_content%?}"

info "Populating default-account..."
replace_after_marker "default_aws_account_id" "- \`${AWS_ACCOUNT_ID}\`"

info "Populating default-region..."
region_content=""
for region in ${DEFAULT_REGIONS}; do
  region_content="${region_content}- \`${region}\`
"
done
replace_after_marker "default_regions" "${region_content%?}"

info "Populating k8s-release-table..."
k8s_content="| Minor Version | Patch Versions |
| --- | --- |
$(echo "${K8S_JSON}" | jq -r '
  .versions[] |
  "| `" + .minor + "` | " + (.patches | map("`" + . + "`") | join(", ")) + " |"
')"
replace_after_marker "k8s_release_table" "${k8s_content}"

info "Populating ami-table..."
if [ "${AMI_COUNT}" -eq 0 ]; then
  ami_content="_No AMIs found._"
else
  ami_rows="$(echo "${AMI_JSON}" | jq -r '
    .items[]? |
    "| `" + (.metadata.name           // "-") + "`" +
    " | `" + (.spec.kubernetesVersion // "-") + "`" +
    " | "  + (.spec.os                // "-") +
    " | "  + (.spec.region            // "-") +
    " | `" + (.spec.imageID           // "-") + "` |"
  ')"
  ami_content="| AMI Name | Kubernetes Version | OS | Region | AMI ID |
| --- | --- | --- | --- | --- |
${ami_rows}"
fi
replace_after_marker "ami-table" "${ami_content}"

# ── Step 5: Missing AMIs ───────────────────────────────────────────────────
# Cross-product of (K8s patch versions × DEFAULT_OS × DEFAULT_REGIONS)
# minus what is already published.

info "Populating missing-ami-table..."
published_lookup=""
if [ "${AMI_COUNT}" -gt 0 ]; then
  published_lookup="$(echo "${AMI_JSON}" | jq -r '
    .items[]? |
    (if (.spec.kubernetesVersion | startswith("v")) then "" else "v" end)
    + .spec.kubernetesVersion + "/" + .spec.os + "/" + .spec.region
  ')"
fi

missing_rows=()
while IFS= read -r raw_version; do
  version="v${raw_version}"
  for os in ${DEFAULT_OS}; do
    for region in ${DEFAULT_REGIONS}; do
      if ! grep -qF "${version}/${os}/${region}" <<< "${published_lookup:-}"; then
        missing_rows+=("| \`${version}\` | ${os} | ${region} |")
      fi
    done
  done
done < <(echo "${K8S_JSON}" | jq -r '.versions[].patches[]')

if [ "${#missing_rows[@]}" -eq 0 ]; then
  missing_content="_No missing AMIs._"
else
  missing_content="| Kubernetes Version | OS | Region |
| --- | --- | --- |
$(printf '%s\n' "${missing_rows[@]}")"
fi
replace_after_marker "missing_ami_table" "${missing_content}"

# ── Step 6: EOL AMIs ──────────────────────────────────────────────────────
# Published AMIs whose minor Kubernetes version is no longer in the K8s table.

info "Populating eol-ami-table..."
supported_minors="$(echo "${K8S_JSON}" | jq -r '.versions[].minor')"

eol_rows=()
if [ "${AMI_COUNT}" -gt 0 ]; then
  while IFS='|' read -r name version os region image_id; do
    minor="$(echo "${version}" | sed 's/^v//' | cut -d'.' -f1,2)"
    if ! grep -qF "${minor}" <<< "${supported_minors}"; then
      eol_rows+=("| \`${name}\` | \`${version}\` | ${os} | ${region} | \`${image_id}\` |")
    fi
  done < <(echo "${AMI_JSON}" | jq -r '
    .items[]? |
    (.metadata.name           // "-") + "|" +
    (.spec.kubernetesVersion  // "-") + "|" +
    (.spec.os                 // "-") + "|" +
    (.spec.region             // "-") + "|" +
    (.spec.imageID            // "-")
  ')
fi

if [ "${#eol_rows[@]}" -eq 0 ]; then
  eol_content="_No EOL AMIs._"
else
  eol_content="| AMI Name | Kubernetes Version | OS | Region | AMI ID |
| --- | --- | --- | --- | --- |
$(printf '%s\n' "${eol_rows[@]}")"
fi
replace_after_marker "eol_ami_table" "${eol_content}"

info "Done — updated ${TARGET_FILE}"
