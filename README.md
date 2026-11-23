# Crusoe Registry Pruner

A single-shot Go job that deletes stale manifests from [Crusoe Container Registry][ccr] (CCR).
It is meant to run as a Kubernetes `CronJob`.

## Background

CCR repositories grow without a bound, and pull-through caches grow fastest.
Every base image a build pulls and every intermediate digest upstream
ever published stays in the registry until somebody deletes it.
CCR has no lifecycle or retention policy, so the storage bill only goes up.

For a pull-through cache, deleting is cheap.
A removed manifest gets re-fetched from upstream the next time something asks for it,
so pruning too aggressively costs one slow pull rather than a lost artifact.
Age-based expiry fits well here: anything nobody has touched in a month is probably not worth paying to store.

Deletion happens on the Crusoe Cloud control-plane API.
This tool talks to `api.cloud.crusoe.ai` through the official Go client SDK.

## What it does

Each run will:

1. List every repository in `CRUSOE_PROJECT_ID`.
2. For each repository, list every image.
3. For each image, list every manifest.
4. Apply the retention policy to each manifest. If it matches, delete the manifest
   by digest, which also removes every tag pointing at it.
5. If `DELETE_IMAGES` is set and every manifest of an image was deleted, delete
   the now-empty image record.
6. Log a summary and exit 0, or exit 1 if anything failed.

### The retention policy

A manifest is deleted when all the following hold, checked in this order:

| Check          | Rule                                                                |
|----------------|---------------------------------------------------------------------|
| Protected tags | None of its tags start with any entry in `KEEP_TAG_PREFIXES`.       |
| Tag state      | It matches `TAG_STATE` (`any`, only `tagged`, or only `untagged`).  |
| Age            | `MAX_AGE` is non-zero and the reference timestamp is older than it. |

`AGE_FROM` picks the reference timestamp:

- `pushed` uses `pushed_at`, when the content entered the registry. This is the default.
- `pulled` uses `pulled_at`, when it was last served.
- `activity` uses whichever of the two is later.

### Behavior worth knowing before you enable it

- **Every repository in the project is in scope.**
  There is no allowlist, denylist, or filter on repository mode,
  so standard read/write repositories get walked alongside pull-through caches.
  Protect anything you publish yourself with `KEEP_TAG_PREFIXES` and `TAG_STATE`,
  or point the job at a project that only holds caches.
- **There is no keep-last-`n` floor.** If every manifest of an image is older than `MAX_AGE`,
  all of them go and the image ends up empty.
- **`MAX_AGE=0` turns off age-based pruning**, which makes the whole run a no-op.
- **`KEEP_TAG_PREFIXES` does prefix matching, not regex**. The prefix `v` keeps `v1.2.3` and `vendor-test`.
- **Deletion is by digest.** A manifest carrying five tags loses all five. The tool never untags a single reference.

### Dry runs

`DRY_RUN=true` gates the API calls in the client.
Nothing is deleted, and each skipped deletion is logged differently.
The summary still counts those as `deleted` and adds their size to freed `bytes`,
so a dry run tells you what a real run would reclaim.

### Output

Structured log output, JSON by default. The last line has the run summary:

```json
{
  "time": "2026-08-10T09:05:19.507754Z",
  "level": "INFO",
  "msg": "finished cleanup",
  "summary": {
    "analyzed": {
      "repositories": 1,
      "images": 8,
      "manifests": 175
    },
    "deleted": {
      "repositories": 0,
      "images": 1,
      "manifests": 15
    },
    "failed": {
      "listings": 0,
      "deletions": 0
    },
    "duration": 9496839334,
    "bytes": 100593087491
  }
}
```

Deletion lines include nested `repository`, `image` and `manifest` groups
with the repository id, name and location, the image name,
and the manifest digest and size.

## Configuration

Configuration is environment-only, parsed under the `CRUSOE_` prefix.
There are no CLI flags and no config file. Enum values are case-insensitive.
Durations use Go syntax (`720h`, `45m`, `1h30m`) and must be non-negative.

| Variable                           | Type                                   | Default  | Description                                                                                               |
|------------------------------------|----------------------------------------|----------|-----------------------------------------------------------------------------------------------------------|
| `CRUSOE_PROJECT_ID`                | `uuid.UUID`                            |          | Crusoe project whose registry gets pruned.                                                                |
| `CRUSOE_ACCESS_KEY`                | `string`                               |          | Crusoe API access key.                                                                                    |
| `CRUSOE_SECRET_KEY`                | `string`                               |          | Crusoe API secret key.                                                                                    |
| `CRUSOE_PRUNER_REQUEST_TIMEOUT`    | `time.Duration`                        | `1m`     | Deadline for each attempt of an API request, including the body read. `0` turns off API request timeouts. |
| `CRUSOE_PRUNER_TIMEOUT`            | `time.Duration`                        | `30m`    | Deadline for the entire run. On expiry the job stops and exits non-zero. `0` removes this deadline.       |
| `CRUSOE_PRUNER_MAX_AGE`            | `time.Duration`                        | `720h`   | Manifests older than this get pruned. `0` turns off age-based pruning, making the run a no-op.            |
| `CRUSOE_PRUNER_AGE_FROM`           | `pushed` \| `pulled` \| `activity`     | `pushed` | The timestamp age is measured from. `activity` uses the later of `pushed_at` and `pulled_at`.             |
| `CRUSOE_PRUNER_TAG_STATE`          | `any` \| `tagged` \| `untagged`        | `any`    | Restrict pruning to tagged or untagged manifests.                                                         |
| `CRUSOE_PRUNER_KEEP_TAG_PREFIXES`  | `[]string`                             | `[]`     | Manifests with any tag starting with one of these prefixes are never pruned. Prefix match, not regex.     |
| `CRUSOE_PRUNER_DELETE_IMAGES`      | `bool`                                 | `false`  | Also delete the image record once all of its manifests have been pruned.                                  |
| `CRUSOE_PRUNER_DRY_RUN`            | `bool`                                 | `false`  | Log what would be deleted without actually deleting.                                                      |
| `CRUSOE_PRUNER_PRUNE_NEVER_PULLED` | `bool`                                 | `false`  | Manifests that have never been pulled get pruned. Only applies when age is measured from `pulled`.        |
| `CRUSOE_PRUNER_LOG_FORMAT`         | `json` \| `text`                       | `json`   | Log handler.                                                                                              |
| `CRUSOE_PRUNER_LOG_LEVEL`          | `debug` \| `info` \| `warn` \| `error` | `info`   | Minimum log level.                                                                                        |
| `CRUSOE_PRUNER_LOG_SOURCE`         | `bool`                                 | `false`  | Include source file and line in log records.                                                              |

### Example

Prune untagged manifests untouched for 14 days, keep anything tagged `v*` or
`release-*`, and clean up the emptied image records:

```dotenv
CRUSOE_PRUNER_MAX_AGE=336h
CRUSOE_PRUNER_AGE_FROM=activity
CRUSOE_PRUNER_TAG_STATE=untagged
CRUSOE_PRUNER_KEEP_TAG_PREFIXES=v,release-
CRUSOE_PRUNER_DELETE_IMAGES=true
```

## Local development

Prerequisites: Go 1.27 or later, [`just`](https://just.systems) 1.55 or later,
and [`pre-commit`](https://pre-commit.com), which is required.
[`golangci-lint`](https://golangci-lint.run) v2 is needed for `just lint` and `just format`,
as the `pre-commit` hooks build their own copy. Docker only if you work with the image,
`helm` and `kubectl` only if you work with the chart.

```sh
git clone https://github.com/jetbrains-research/crusoe-registry-pruner
cd crusoe-registry-pruner
just setup
```

Run `just` on its own to list every recipe.

Configuration is environment-only, so local runs read from a `.env` file you create yourself:

```dotenv
CRUSOE_PROJECT_ID=...
CRUSOE_ACCESS_KEY=...
CRUSOE_SECRET_KEY=...

CRUSOE_PRUNER_LOG_FORMAT=text
CRUSOE_PRUNER_LOG_LEVEL=debug
CRUSOE_PRUNER_MAX_AGE=720h
```

Run it:

```sh
just run        # dry run
just run false  # deletes for real
```

`just run` loads `.env` and sets `CRUSOE_PRUNER_DRY_RUN` from its argument,
so any `DRY_RUN` value in `.env` is ignored.

The `pre-commit` hooks format the code and lint it with [`golangci-lint`](https://golangci-lint.run),
using the configuration in `.golangci.yaml`. To run them against the whole repository:

```sh
just hook
```

Checks before pushing:

```sh
just lint
just format
just vet
just test
just lock
```

`just lint` and `just format` rewrite files in place.
`just test` takes an optional package pattern and extra `go test` flags,
for example `just test ./internal/config/... -v`.

Dependencies:

```sh
just upgrade
just tidy
```

### Building

```sh
just binary # writes bin/crusoe-registry-pruner
just build  # container image
```

Both stamp the version from `git describe`.
The image is a static binary on distroless.
To pass extra arguments during `docker build`:

```sh
just build --platform linux/arm64
```

To render the chart from source, pass extra arguments to `helm template`:

```sh
just template --set image.tag=`git describe --tags --always --dirty`
```

The chart in the repository only gets its real version on release,
so set `image.tag` when rendering it from source.

## Deploying the Helm chart

Every release publishes the chart to GHCR as an OCI artifact at
`oci://ghcr.io/jetbrains-research/charts/crusoe-registry-pruner`.
The chart version is the release tag without the `v`, so release `v0.3.2` ships chart `0.3.2`.
It does not manage credentials. You create the `Secret` yourself.

### Create the credentials `Secret`

The chart mounts it with `envFrom.secretRef`,
so the keys have to be named exactly like the environment variables,
and the `Secret` has to live in the release namespace.

```sh
kubectl create secret generic crusoe-secrets \
  --from-literal=CRUSOE_ACCESS_KEY=$CRUSOE_ACCESS_KEY \
  --from-literal=CRUSOE_API_ENDPOINT=$CRUSOE_API_ENDPOINT \
  --from-literal=CRUSOE_PROJECT_ID=$CRUSOE_PROJECT_ID \
  --from-literal=CRUSOE_SECRET_KEY=$CRUSOE_SECRET_KEY \
  --namespace crusoe-system
```

On a CMK cluster, the auto-provisioned secret may already carry `CRUSOE_ACCESS_KEY` and `CRUSOE_SECRET_KEY`.
Check with:

```sh
kubectl get secret -n crusoe-system
```

Before minting a new key pair. The chart takes a single `secretName`,
so whichever `Secret` you point it at needs all three keys.

The Crusoe API key pair is project-wide, not registry-scoped.
If your organization can mint restricted keys, use one here.

### Write a values file

```yaml
schedule: "0 3 * * *" # 03:00 daily (cluster timezone)

config:
  maxAge: "336h"
  ageFrom: activity
  tagState: untagged
  keepTagPrefixes:
    - v
    - release-
  deleteImages: true
  dryRun: true # flip to false once the logs look right
```

Empty strings and `false` under `config` are left out of the `ConfigMap` entirely, so the binary's own defaults apply.
A `0` duration is kept, so `maxAge: 0` turns off age-based pruning and `timeout: 0` removes the deadline.
Set only what you want to override.

Leave `image.tag` unset. Each chart version defaults to the image from the same release.

To see every value with its default:

```sh
helm show values oci://ghcr.io/jetbrains-research/charts/crusoe-registry-pruner --version 0.3.2
```

### Install

```sh
helm upgrade --install crusoe-registry-pruner oci://ghcr.io/jetbrains-research/charts/crusoe-registry-pruner \
  --version 0.3.2 \
  --namespace crusoe-system \
  --values values.prod.yaml \
  --create-namespace
```

Without `--version`, Helm installs the latest release.
Render without applying to check the output first:

```sh
helm template crusoe-registry-pruner oci://ghcr.io/jetbrains-research/charts/crusoe-registry-pruner \
  --version 0.3.2 \
  --namespace crusoe-system \
  --values values.prod.yaml
```

### Trigger a run without waiting for the schedule

To create a one-off `Job`:

```sh
kubectl create job --from=cronjob/crusoe-registry-pruner crusoe-registry-pruner-test --namespace crusoe-system
```

You can view logs of all jobs with:

```sh
kubectl logs -n crusoe-system -l app.kubernetes.io/name=crusoe-registry-pruner --tail=-1
```

### Chart values

| Value                      | Default                                                         | Description                                                                                                               |
|----------------------------|-----------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| `schedule`                 | `"0 0 * * *"`                                                   | `CronJob` schedule, in the cluster timezone.                                                                              |
| `secretName`               | `"crusoe-secrets"`                                              | Existing `Secret` holding `CRUSOE_PROJECT_ID`, `CRUSOE_ACCESS_KEY` and `CRUSOE_SECRET_KEY`. The chart does not create it. |
| `image.repository`         | `ghcr.io/jetbrains-research/crusoe-registry-pruner`             | Image repository.                                                                                                         |
| `image.tag`                | `""`                                                            | Image tag. Falls back to `v{{ .Chart.AppVersion }}`, the image from the same release.                                     |
| `image.pullPolicy`         | `IfNotPresent`                                                  | Image pull policy.                                                                                                        |
| `imagePullSecrets`         | `[]`                                                            | Pull secrets for a private registry.                                                                                      |
| `concurrencyPolicy`        | `Forbid`                                                        | Prevents overlapping runs.                                                                                                |
| `restartPolicy`            | `Never`                                                         | Pod restart policy.                                                                                                       |
| `priorityClassName`        | Default priority or `0`.                                        | Pod scheduling priority.                                                                                                  |
| `historyLimits.successful` | `3`                                                             | Number of succeeded `Job` resources to keep after a run finishes.                                                         |
| `historyLimits.failed`     | `3`                                                             | Number of failed `Job` resources to keep after a run finishes.                                                            |
| `resources`                | 50m CPU and 32Mi memory requested, 64Mi memory limit.           | Container resources.                                                                                                      |
| `securityContext`          | All capabilities dropped with non-root user and read-only root. | Container security context.                                                                                               |
| `extraEnv`                 | `[]`                                                            | Extra `env` entries appended to the container.                                                                            |
| `config.*`                 | See the table above                                             | Maps to `CRUSOE_PRUNER_*`. Empty and false values are omitted.                                                            |
| `log.*`                    | See the table above.                                            | Maps to `CRUSOE_PRUNER_LOG_*`. Always emitted.                                                                            |

## License

See [LICENSE](LICENSE).

[ccr]: https://docs.crusoecloud.com/container-registry/overview/index.html
