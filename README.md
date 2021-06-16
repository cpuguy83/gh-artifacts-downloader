# Github Actions Artifact Downloader

This is a small tool that downloads artifacts from GitHub Actions workflow runs.

# Install

Build with go

```terminal
$ go install github.com/cpuguy83/gh-actions-downloader@latest
```

Build with Docker:

```terminal
$ docker build --output=bin/ --platform=local https://github.com/cpuguy83/gh-artifacts-downloader.git#v0.2.0
```

## Usage

Download all artifacts that contain `TestResults` in the name from the `containerd/containerd` repo where the action was triggered on the `master` branch due to a `push` event *after* the workflow run id `925745732` and put them into the `out` dir.

```terminal
$ export GITHUB_TOKEN=XXXXXXXXXXX
$ export GITHUB_USER=cpuguy83
$ ./gh-artifacts-downloader --repo containerd/containerd --branch-filter=master --event-filter=push --output=out --patern TestResults --id 925745732
```

To download artifacts from a single workflow run add `--single` to the command line.