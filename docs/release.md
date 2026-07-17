# Release process

Releases are created from `v*` tags whose commits are contained in `main`. The workflow rejects tags from other branches.

1. Ensure `main` is clean and up to date.
2. Run the local validation:

   ```sh
   go test ./...
   go vet ./...
   VERSION=v0.3.0 scripts/build-release.sh
   (cd dist && LC_ALL=C LANG=C shasum -a 256 -c SHA256SUMS)
   ```

3. Push `main`, then create and push the version tag:

   ```sh
   git push origin main
   git tag v0.3.0
   git push origin v0.3.0
   ```

The release workflow rebuilds all archives, creates `SHA256SUMS`, and publishes the archives plus `install.sh` to a GitHub Release.

After the workflow succeeds, verify the public installer in a temporary directory:

```sh
curl -fsSL https://github.com/qiz029/vibe-pushover/releases/download/v0.3.0/install.sh | \
  VIBE_PUSHOVER_VERSION=v0.3.0 \
  VIBE_PUSHOVER_INSTALL_DIR=/tmp/vibe-pushover-install-smoke \
  sh

/tmp/vibe-pushover-install-smoke/vibe-pushover --version
```
