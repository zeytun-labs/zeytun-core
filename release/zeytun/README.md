# Zeytun desktop core releases

The app consumes releases built here; it never compiles Go in its CI or release
pipeline. Push a new `zeytun-core-v*` tag at a reviewed source commit to build
macOS Apple Silicon and Windows x64 binaries. Both use the production feature
sets in `release/DEFAULT_BUILD_TAGS*`, plus gRPC and the V2Ray API. macOS uses
CGO; Windows uses purego and ships the matching pinned Cronet DLL.

The workflow runs each executable's version and configuration checks before
publishing a complete prerelease. It includes license/notice files, build
metadata, SHA256SUMS, the exact corresponding source archive, and
`core-release.json`. Releases are never updated in place: corrections receive a
new tag. Copy the manifest into the app's `scripts/core-release.json` in a
reviewed dependency update. Never select `latest` or a moving branch in app CI.

Only the publisher job can write repository contents. Publishing requires both
native builds to pass. The upstream multi-platform Build workflow is separate
and is not invoked by these tags. Client submodules are not used to compile the
Go command; Go dependencies are locked by go.mod/go.sum.
