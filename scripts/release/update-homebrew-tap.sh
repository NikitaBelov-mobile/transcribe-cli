#!/usr/bin/env bash
set -euo pipefail

: "${HOMEBREW_TAP_REPO:?HOMEBREW_TAP_REPO is required}"
: "${HOMEBREW_TAP_TOKEN:?HOMEBREW_TAP_TOKEN is required}"
: "${RELEASE_REPO:?RELEASE_REPO is required}"

TAG="${GITHUB_REF_NAME:-}"
if [[ -z "$TAG" ]]; then
  echo "GITHUB_REF_NAME is empty"
  exit 1
fi
VERSION="${TAG#v}"
CHECKSUMS_FILE="dist/checksums.txt"
if [[ ! -f "$CHECKSUMS_FILE" ]]; then
  echo "missing $CHECKSUMS_FILE"
  exit 1
fi

sha_for() {
  local artifact="$1"
  awk -v name="$artifact" '$2 == name { print $1 }' "$CHECKSUMS_FILE"
}

DARWIN_AMD64="transcribe-cli_${VERSION}_darwin_amd64.zip"
DARWIN_ARM64="transcribe-cli_${VERSION}_darwin_arm64.zip"
SHA_AMD64="$(sha_for "$DARWIN_AMD64")"
SHA_ARM64="$(sha_for "$DARWIN_ARM64")"

if [[ -z "$SHA_AMD64" || -z "$SHA_ARM64" ]]; then
  echo "failed to find darwin checksums in $CHECKSUMS_FILE"
  exit 1
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

git clone "https://x-access-token:${HOMEBREW_TAP_TOKEN}@github.com/${HOMEBREW_TAP_REPO}.git" "$WORKDIR/tap"
mkdir -p "$WORKDIR/tap/Formula"

cat > "$WORKDIR/tap/Formula/transcribe.rb" <<FORMULA
class Transcribe < Formula
  desc "Offline transcription CLI for audio/video"
  homepage "https://github.com/${RELEASE_REPO}"
  version "${VERSION}"

  on_macos do
    if Hardware::CPU.intel?
      url "https://github.com/${RELEASE_REPO}/releases/download/${TAG}/${DARWIN_AMD64}"
      sha256 "${SHA_AMD64}"
    end

    if Hardware::CPU.arm?
      url "https://github.com/${RELEASE_REPO}/releases/download/${TAG}/${DARWIN_ARM64}"
      sha256 "${SHA_ARM64}"
    end
  end

  def install
    bin.install "transcribe"
  end

  test do
    assert_match "transcribe - offline transcription CLI", shell_output("#{bin}/transcribe help")
  end
end
FORMULA

pushd "$WORKDIR/tap" >/dev/null
git add Formula/transcribe.rb
if git diff --cached --quiet; then
  echo "homebrew formula unchanged"
  exit 0
fi
git commit -m "transcribe ${VERSION}"
git push origin HEAD
popd >/dev/null
