# detect-k8s-release

A small Go tool that fetches the latest stable Kubernetes release tags from
[kubernetes/kubernetes](https://github.com/kubernetes/kubernetes) and writes a
structured JSON file listing the three most recent minor versions together with
all their patch releases.

The three-minor-version window follows the
[CAPA AMI publication policy](https://cluster-api-aws.sigs.k8s.io/topics/images/built-amis#ami-publication-policy).

## Output

The tool writes to `data/k8s-supported-versions.json`:

```json
{
  "generated_at": "2026-04-24T00:00:00Z",
  "versions": [
    { "minor": "1.32", "patches": ["1.32.3", "1.32.2", "1.32.1", "1.32.0"] },
    { "minor": "1.31", "patches": ["1.31.7", "1.31.6"] },
    { "minor": "1.30", "patches": ["1.30.11"] }
  ]
}
```

## Prerequisites

- Go 1.24 or later
- A GitHub personal access token is **recommended** to avoid the unauthenticated
  API rate limit (60 requests/hour). Generate one at
  <https://github.com/settings/tokens> — no scopes are required for public
  repository access.

## Running locally

From the `hack/tools/detect-k8s-release/` directory:

```bash
# Without a token (rate-limited to 60 requests/hour)
go run .

# With a token (recommended)
go run . --token "$GITHUB_TOKEN"
```

The tool prints the detected versions and writes the JSON file:

```
Fetching Kubernetes release tags from GitHub...
  minor 1.32: 4 patch release(s)
  minor 1.31: 2 patch release(s)
  minor 1.30: 1 patch release(s)
Written to data/k8s-supported-versions.json
```

## Running tests

```bash
go test ./...
```

## Automation

This tool is executed automatically by the
`.github/workflows/detect-k8s-releases.yml` workflow, which runs at midnight
UTC on the **1st and 15th of each month** (aligned with the Kubernetes monthly
patch release cadence). When the JSON file changes the workflow opens a pull
request to update it in the repository.

You can also trigger the workflow manually from the
[Actions tab](https://github.com/kubernetes-sigs/cluster-api-provider-aws/actions/workflows/detect-k8s-releases.yml).
