# Releasing

This guide is for maintainers. Releases are published by CI when a new `v*`
version tag is pushed.

## Validate locally

Install GoReleaser, then check the configuration and create a local snapshot:

```sh
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser check
goreleaser release --snapshot --clean
```

Snapshot artifacts are written to `dist/`; they are not published.

## Publish a release

Make sure the release changes are pushed and CI passes. Choose the next
appropriate semantic version and ensure its tag does not already exist. Replace
`vX.Y.Z` below with that new version:

```sh
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

Pushing the tag triggers the release job in
[`.github/workflows/ci.yml`](./.github/workflows/ci.yml). GoReleaser builds
archives for Linux, macOS, and Windows on amd64 and arm64, creates checksums, and
publishes a GitHub Release. The workflow uses GitHub's provided token with
repository contents write permission; no personal access token is needed.
