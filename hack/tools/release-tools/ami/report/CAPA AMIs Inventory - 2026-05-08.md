## Kubernetes Release

This section lists the Kubernetes versions tracked by this repository following the [CAPA AMI publication policy](https://cluster-api-aws.sigs.k8s.io/topics/images/built-amis#ami-publication-policy): the latest three supported minor releases and all their stable patch versions as detected by `release-tool ami detect-k8s-release`.

| Minor Version | Patch Versions |
| --- | --- |
| `1.36` | `1.36.0` |

## CAPA AMI

The table below lists all AMIs currently published in AWS account `027487054958`, as returned by `clusterawsadm ami list --owner-id 027487054958`.

| AMI Name | Kubernetes Version | OS | Region | AMI ID | Created |
| --- | --- | --- | --- | --- | --- |
| `capa-ami-ubuntu-24.04-v1.35.2-1773581120` | `v1.35.2` | ubuntu-24.04 | ap-southeast-2 | `ami-065d95b98ad687175` | 2026-03-15T13:36:47Z |
| `capa-ami-ubuntu-22.04-v1.35.2-1773582653` | `v1.35.2` | ubuntu-22.04 | ap-southeast-2 | `ami-04136e76627f61a0b` | 2026-03-15T14:04:21Z |
| `capa-ami-ubuntu-22.04-v1.35.1-1773640135` | `v1.35.1` | ubuntu-22.04 | ap-southeast-2 | `ami-01da8306336074553` | 2026-03-16T06:01:02Z |

## Missing AMI

### Default OS

List of OS for which AMIs should be published (default):

- ubuntu-24.04

### Default Region

List of regions for which AMIs should be published (default):

- ap-southeast-2

### List of Missing AMIs

| Kubernetes Version | OS | Region |
| --- | --- | --- |
| `v1.36.0` | ubuntu-24.04 | ap-southeast-2 |

