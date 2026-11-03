---
title: Maintaining
weight: 2
---

For maintainers: version bumps and releases. Run the scripts with mise, which puts the pinned GoReleaser and minisign on `PATH`: `mise exec -- scripts/release.sh`.

## Updating pinned versions

- `scripts/bump.sh --check` lists every pin against upstream; `scripts/bump.sh [tool...]` rewrites the pins (all tools by default) and prints a summary for the commit message. It writes a value only if it has the shape of a pin (a version, a 64-hex checksum, an image digest, a dated release, a commit), since some of the edited files run on the host, and edits nothing unless every value is good. It needs `curl` and `jq`, and `apt` needs `gpgv`.
- The tools: `apt`, `docker`, `mise`, `rustup`, `crane`, `omb`, `eza`, `tealdeer`. `apt` moves `apt_snapshot` (base's packages) to today, and with it the images base starts from, all in `internal/ubuntu/ubuntu.go`: the digest of Ubuntu's container image, smolvm's and Docker's (`ContainerDigest`), refusing an image built after the snapshot, whose packages would be newer than the snapshot's (try the next day); and Ubuntu's cloud image, Lima's (`CloudRelease`, `CloudSHA256Arm64`, `CloudSHA256Amd64`): the newest dated release before the snapshot's date on cloud-images.ubuntu.com, with its images' SHA-256s from its `SHA256SUMS`, which `gpgv` must find signed by `assets/layers/ubuntu-cloud.gpg` (it refuses when no release is old enough, or the signature fails). Ubuntu removes dated releases after a few months, and the pinned one going stops Lima's base builds: bump `apt` well before that; `docker` moves Docker's packages (`assets/layers/docker.sh`), read from its repository for base's Ubuntu release, and its signing key. Packages added to `assets/layers/base.sh` or `assets/features/` need no pin of their own: the snapshot covers them.
- `mise` moves base's mise (`assets/layers/base.sh`) and devvm's host mise (`internal/tools/mise.go`) together: the host one writes the lock base's installs from.
- The shared tools aren't pinned: holds go in `assets/shared-tools/mise.toml` (`node = "24"` stays on that major). `testdata/mise.lock` is a fixed lock for the tests.
- `assets/layers/ubuntu-cloud.gpg` is Ubuntu's cloud image signing key (fingerprint `D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81`), from keyserver.ubuntu.com. If Ubuntu changes it, base builds stop with `ubuntu_bad_signature`: fetch the new key, check its fingerprint against Ubuntu's announcement, and replace the file (`gpg --export FPR`).
- Edited scripts change image keys, so the affected images rebuild on next use. Build them in a sandbox of their own (`LIMA_HOME`, `DEVVM_LIMA_HOME`, `XDG_DATA_HOME`, `XDG_CONFIG_HOME`) before committing (`devvm build docker`, or a VM made with the tools whose build script changed), then `devvm gc`, and run the [end-to-end tests](_index.md#end-to-end-tests).

## Releasing

- **Once, the keys:** a minisign key for releases, `minisign -G -p packaging/minisign.pub -s ~/.minisign/devvm.key` (commit `packaging/minisign.pub`; the secret key and its password never go in the repo), and a key git signs tags with (`git config user.signingkey`, GPG, or SSH with `gpg.format ssh`) that `git tag -v` verifies on this machine. Commit its fingerprint in `packaging/release-signer`, one per line (`#` comments): the GPG fingerprint (`gpg --fingerprint`, no spaces), or the SSH key's `SHA256:…` (`ssh-keygen -lf KEY.pub`). `release.sh` refuses a tag signed by any other key, even one git trusts. To rotate, add the new key's line, tag with it, then remove the old one.
- Bump `VERSION`, add the `CHANGELOG.md` entry, commit, then `git tag -s v<version>`.
- `scripts/release.sh` checks the tag with `git tag -v` and its signer against this checkout's `packaging/release-signer`, then builds `dist/` from the tagged commit with GoReleaser (`.goreleaser.yaml`), in a clone of its own, so only what's committed goes in. It builds in two passes: first the Linux binaries alone (`goreleaser build --id devvm`), whose SHA-256s it takes; then the release (`goreleaser release --skip=publish`), whose macOS builds get those SHA-256s and the Linux archives' addresses (`DEVVM_RELEASE_URL`, else the GitHub release's) with `-X`, for the Linux build [Docker](../backends/docker.md#requirements) fetches. It refuses a release whose Linux binaries came out different in the second pass (Go's builds are reproducible: the same commit, Go and flags), or whose Linux archives' devvm isn't the pinned one. It checks that the binary built for this host prints the version, then signs `SHA256SUMS` with `~/.minisign/devvm.key` (or `DEVVM_MINISIGN_KEY`) and verifies it with `packaging/minisign.pub`. Nothing is published.
- `dist/` then holds:

  | File | What |
  | --- | --- |
  | `devvm-<version>-<os>-<arch>.tar.gz` | the binary, its files (`assets/`) embedded, with `README.md`, `CHANGELOG.md` and `LICENSE`, for `darwin` and `linux`, `arm64` and `amd64`; `darwin`'s binaries carry their processor's `linux` archive's address and the SHA-256 of the devvm in it, which they fetch and check for [Docker](../backends/docker.md#requirements) |
  | `devvm-<version>.tar.gz` | the source, which the Homebrew formula builds from |
  | `SHA256SUMS`, `SHA256SUMS.minisig` | their checksums, and the signature |
  | `install.sh` | the install script, set to this release and to `packaging/minisign.pub`'s key |
  | `devvm.rb` | the Homebrew formula, set to the source tarball's URL and checksum |

- `scripts/release.sh --head` builds from HEAD's commit instead, for testing, with `VERSION`'s version. It's unsigned unless `DEVVM_MINISIGN_KEY` names a test key; `DEVVM_MINISIGN_PUB` names its public key in place of `packaging/minisign.pub`. `DEVVM_RELEASE_URL` points the formula, and the macOS builds' Linux archives, at a test location. A commit without `install.sh` is refused.
- To try `dist/install.sh` on such a build, copy `dist/` to `DIR/releases/download/v<version>/`, then in a throwaway `HOME` with `DEVVM_PREFIX` under it: `DEVVM_INSTALL_TEST=1 source dist/install.sh`, set `repo_url=file://DIR` and `protos='=https,file'` (it downloads over HTTPS only), and run `main --no-deps`. Without `--no-deps` it offers to install what the host lacks: answer no, or run it with no terminal.
- Every build sets the version with `-X github.com/w3cj/devvm/internal/cli.version=<version>`: devvm doesn't read `VERSION`. The formula (`packaging/homebrew/devvm.rb`) builds with Go the same way, installs the binary alone (on macOS with the Linux build in `libexec`), and passes `brew audit --strict`.
- Attach the archives, `SHA256SUMS`, `SHA256SUMS.minisig` and `install.sh` to the GitHub release; `devvm.rb` goes in `w3cj/homebrew-tap` as `Formula/devvm.rb`. Publishing is the maintainer's call: ask first.
