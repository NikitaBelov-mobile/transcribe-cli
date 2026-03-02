#!/usr/bin/env bash
set -euo pipefail

: "${WINGET_REPO:?WINGET_REPO is required}"
: "${WINGET_REPO_TOKEN:?WINGET_REPO_TOKEN is required}"
: "${WINGET_PACKAGE_ID:?WINGET_PACKAGE_ID is required}"
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

WIN_AMD64="transcribe-cli_${VERSION}_windows_amd64.zip"
WIN_ARM64="transcribe-cli_${VERSION}_windows_arm64.zip"
SHA_AMD64="$(sha_for "$WIN_AMD64")"
SHA_ARM64="$(sha_for "$WIN_ARM64")"

if [[ -z "$SHA_AMD64" || -z "$SHA_ARM64" ]]; then
  echo "failed to find windows checksums in $CHECKSUMS_FILE"
  exit 1
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

git clone "https://x-access-token:${WINGET_REPO_TOKEN}@github.com/${WINGET_REPO}.git" "$WORKDIR/winget"

first_char="$(echo "$WINGET_PACKAGE_ID" | cut -c1 | tr '[:upper:]' '[:lower:]')"
manifest_dir="$WORKDIR/winget/manifests/${first_char}/${WINGET_PACKAGE_ID}/${VERSION}"
mkdir -p "$manifest_dir"

cat > "$manifest_dir/${WINGET_PACKAGE_ID}.yaml" <<VERSION_MANIFEST
PackageIdentifier: ${WINGET_PACKAGE_ID}
PackageVersion: ${VERSION}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
VERSION_MANIFEST

cat > "$manifest_dir/${WINGET_PACKAGE_ID}.installer.yaml" <<INSTALLER_MANIFEST
PackageIdentifier: ${WINGET_PACKAGE_ID}
PackageVersion: ${VERSION}
Installers:
  - Architecture: x64
    InstallerType: zip
    NestedInstallerType: portable
    NestedInstallerFiles:
      - RelativeFilePath: transcribe.exe
        PortableCommandAlias: transcribe
    InstallerUrl: https://github.com/${RELEASE_REPO}/releases/download/${TAG}/${WIN_AMD64}
    InstallerSha256: ${SHA_AMD64}
  - Architecture: arm64
    InstallerType: zip
    NestedInstallerType: portable
    NestedInstallerFiles:
      - RelativeFilePath: transcribe.exe
        PortableCommandAlias: transcribe
    InstallerUrl: https://github.com/${RELEASE_REPO}/releases/download/${TAG}/${WIN_ARM64}
    InstallerSha256: ${SHA_ARM64}
ManifestType: installer
ManifestVersion: 1.6.0
INSTALLER_MANIFEST

cat > "$manifest_dir/${WINGET_PACKAGE_ID}.locale.en-US.yaml" <<LOCALE_MANIFEST
PackageIdentifier: ${WINGET_PACKAGE_ID}
PackageVersion: ${VERSION}
PackageLocale: en-US
Publisher: Nikita
PackageName: transcribe-cli
License: MIT
ShortDescription: Offline transcription CLI for audio and video files
ManifestType: defaultLocale
ManifestVersion: 1.6.0
LOCALE_MANIFEST

pushd "$WORKDIR/winget" >/dev/null
git add "$manifest_dir"
if git diff --cached --quiet; then
  echo "winget manifests unchanged"
  exit 0
fi
git commit -m "${WINGET_PACKAGE_ID} ${VERSION}"
git push origin HEAD
popd >/dev/null
