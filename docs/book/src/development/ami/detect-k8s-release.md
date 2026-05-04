# Detect Kubernetes Release Versions

CAPA's [AMI publication policy][ami-policy] tracks the latest three minor
Kubernetes versions and their stable patch releases. The `release-tool`
binary automates that lookup against the upstream
[`kubernetes/kubernetes`](https://github.com/kubernetes/kubernetes) GitHub
repository, so maintainers (and the [`detect-k8s-releases` GitHub Actions
workflow][workflow]) never have to hand-curate the list.

This guide covers what the tool produces, how to build and run it locally,
the available flags, and how it is wired into CI.

[ami-policy]: ../../topics/images/built-amis.md#ami-publication-policy
[workflow]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/main/.github/workflows/detect-k8s-releases.yml

## What the tool does

`release-tool ami detect-k8s-release` calls the GitHub REST API
(`GET /repos/kubernetes/kubernetes/tags`) with pagination and filtering, then
emits a structured list of supported minors and their patch versions, sorted
newest first.

By default it returns the **latest 3 minor versions** (the CAPA AMI policy).
You can override the count with `--latest-version-count`, or request specific
minors explicitly with `--version`.

The output is consumed by the [detect-k8s-releases workflow][workflow], which
writes it to `hack/tools/release-tools/ami/k8srelease/data/AMIBuildConfig.json`
and opens an automated pull request whenever the contents change.

## Where it lives

The tool is a small standalone Go module under `hack/tools/release-tools/`:

```
hack/tools/release-tools/
├── main.go
├── go.mod
├── cmd/                                 # release-tool root command
├── ami/
│   ├── cmd/                             # ami subcommand group
│   │   └── k8srelease/                  # detect-k8s-release cobra leaf
│   └── k8srelease/                      # business logic
│       └── data/AMIBuildConfig.json     # generated output (gitignored locally)
└── printer/                             # table/json/yaml renderers
```

It is intentionally kept as its own Go module so its dependency surface
(`spf13/cobra`, `google/go-github`, `blang/semver`, `golang.org/x/oauth2`,
`sigs.k8s.io/yaml`) does not leak into the main CAPA module.

## Build it locally

From the repository root:

```bash
( cd hack/tools/release-tools && go build -o ../../../bin/release-tool . )
```

The binary lands at `bin/release-tool`. The CLI command name is intentionally
singular even though the directory is `release-tools/`.

## Usage

```
release-tool ami detect-k8s-release [flags]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--token` | `$GITHUB_TOKEN` env var, otherwise `""` | GitHub personal access token. Optional, but raises the rate limit from 60 req/h (unauthenticated) to 5,000 req/h. The token does **not** need any scopes — public repo tags are readable without auth. The flag takes precedence over the environment variable. |
| `--latest-version-count` | `3` | Number of latest minor Kubernetes versions to return. Mutually exclusive with `--version`. |
| `--version` | `""` | Comma-separated list of explicit MAJOR.MINOR versions to fetch (e.g. `1.34,1.30`). May be specified at most once; mutually exclusive with `--latest-version-count`. |
| `--output`, `-o` | `table` | Output format: `table`, `json`, or `yaml`. |

#### Token precedence

1. `--token X` on the command line — highest priority.
2. `GITHUB_TOKEN` environment variable — automatic fallback.
3. Unauthenticated — last resort, subject to GitHub's 60 req/h IP rate limit.

### Mutual-exclusion rules

The CLI rejects ambiguous combinations before making any API calls:

- `--version` and `--latest-version-count` cannot both be supplied.
- Each flag may only be specified once on the command line. Use the
  comma-separated form for `--version` (e.g. `--version 1.34,1.30`) — the
  repeated form (`--version 1.34 --version 1.30`) is rejected.

### Examples

```bash
# Default — latest 3 minor versions, table output.
# GITHUB_TOKEN is picked up from the environment automatically.
export GITHUB_TOKEN=ghp_xxx
./bin/release-tool ami detect-k8s-release
```

```text
MINOR VERSION  PATCH VERSIONS
1.36           1.36.0
1.35           1.35.4, 1.35.3, 1.35.2, 1.35.1, 1.35.0
1.34           1.34.7, 1.34.6, 1.34.5, 1.34.4, 1.34.3, 1.34.2, 1.34.1, 1.34.0
```

```bash
# Latest 6 minor versions, JSON
./bin/release-tool ami detect-k8s-release --latest-version-count 6 --output json
```

```bash
# Specific minors, YAML
./bin/release-tool ami detect-k8s-release --version 1.34,1.30 -o yaml
```

```bash
# Override the token explicitly (takes precedence over $GITHUB_TOKEN)
./bin/release-tool ami detect-k8s-release --token ghp_yyy
```

```yaml
versions:
- minor: "1.34"
  patches:
  - 1.34.7
  - 1.34.6
  ...
- minor: "1.30"
  patches:
  - 1.30.14
  - 1.30.13
  ...
```

### Output schema

JSON and YAML share the same shape:

```json
{
  "versions": [
    {
      "minor": "1.36",
      "patches": ["1.36.0"]
    },
    {
      "minor": "1.35",
      "patches": ["1.35.4", "1.35.3", "1.35.2", "1.35.1", "1.35.0"]
    }
  ]
}
```

The output is **content-deterministic** — identical inputs produce
byte-identical output. There is no embedded timestamp, so the workflow only
opens a PR when an actual Kubernetes version changes.

### Filtering rules

- Only stable tags matching `^v1\.(\d+)\.(\d+)$` are considered. Pre-release
  suffixes (`-alpha.N`, `-beta.N`, `-rc.N`) are filtered out.
- Versions are compared with [`blang/semver`][blang-semver]; minors and
  patches are sorted newest first.
- For explicit `--version` inputs, the leading `v` is optional (`v1.34` and
  `1.34` are both accepted).

[blang-semver]: https://pkg.go.dev/github.com/blang/semver

### Exit codes & errors

| Condition | Behaviour |
|---|---|
| Ambiguous flag combination (e.g. duplicate `--version`) | Exit 1, error message before any API call. |
| Unknown minor (`--version 1.99`) | Exit 1, `unknown Kubernetes version "1.99"`. |
| GitHub rate limit exceeded | Exit 1, error returned by `go-github` includes reset time. |
| Network failure | Exit 1, error wrapped with `listing kubernetes/kubernetes tags: ...`. |

## CI integration

The [`detect-k8s-releases` workflow][workflow] runs the tool and opens a PR
whenever the generated config changes:

```yaml
- name: Build release-tool
  working-directory: hack/tools/release-tools
  run: go build -o ../../../bin/release-tool .

- name: Detect latest K8s releases (CAPA policy)
  env:
    # The CLI picks this up automatically; no --token flag required.
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    mkdir -p hack/tools/release-tools/ami/k8srelease/data
    ./bin/release-tool ami detect-k8s-release --output json \
      > hack/tools/release-tools/ami/k8srelease/data/AMIBuildConfig.json

- name: Open PR if versions changed
  uses: peter-evans/create-pull-request@v8
  with:
    branch: automation/update-k8s-supported-versions
    add-paths: hack/tools/release-tools/ami/k8srelease/data/AMIBuildConfig.json
    ...
```

The workflow can be triggered manually via `workflow_dispatch`, or scheduled
to run on a cron (currently commented out, intended to align with the
Kubernetes monthly patch release cadence).

> **Note on CI for the auto-PR.** The default `secrets.GITHUB_TOKEN` cannot
> trigger downstream workflows on the PRs it creates — this is a GitHub-wide
> rule, not specific to this tool. To make the auto-PR run lint and unit
> tests automatically, replace the token with a Personal Access Token or a
> GitHub App token.

## Adding a new release-tool subcommand

The CLI is structured so new subcommands slot in without touching existing
ones. To add e.g. `release-tool ami list-images`:

1. Create the business-logic package: `hack/tools/release-tools/ami/listimages/listimages.go`.
2. Create the cobra leaf: `hack/tools/release-tools/ami/cmd/listimages/listimages.go`
   that imports the business-logic package and `printer`.
3. Add one line to `hack/tools/release-tools/ami/cmd/ami.go`:
   `cmd.AddCommand(listimagescmd.Cmd())`.

For a sibling domain (e.g. `release-tool releaseversion ...`), repeat the
pattern at the top level: create `releaseversion/...` and register it in
`hack/tools/release-tools/cmd/root.go`.
