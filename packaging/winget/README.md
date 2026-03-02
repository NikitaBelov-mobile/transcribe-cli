# Winget Publishing

This project can auto-publish manifests via `.github/workflows/release.yml`.

Required repository variable and secrets:

- `vars.RELEASE_REPO` (example: `your-org/transcribe-cli`)
- `vars.WINGET_REPO` (manifest repo to push into)
- `vars.WINGET_PACKAGE_ID` (example: `YourOrg.TranscribeCLI`)
- `secrets.WINGET_REPO_TOKEN` (PAT with push access to `WINGET_REPO`)

On every `v*` tag release, the workflow generates manifests for:

- `windows_amd64`
- `windows_arm64`

and pushes them to the winget manifests repository.
