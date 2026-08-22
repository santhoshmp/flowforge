# FlowForge — Release Runbook (P3)

| | |
|---|---|
| **Trigger** | `git tag vX.Y.Z && git push origin vX.Y.Z` |
| **Pipeline** | `.github/workflows/release.yml` |
| **Outputs** | 6 binary archives + `SHA256SUMS` + SPDX SBOM + cosign signatures + `ghcr.io/flowforge/flowforge` image + GitHub Release |

## 1. What the pipeline does

1. **UI build** — `dsl` (contract) then `app` (Vite) → copied into the Go embed.
2. **Cross-compile matrix** (`scripts/build.sh`) — linux/darwin/windows ×
   amd64/arm64, `CGO_ENABLED=0`, `-trimpath`, version stamped via ldflags.
   Packaged as `.tar.gz` (unix) / `.zip` (windows) + `SHA256SUMS`.
3. **SBOM** — syft SPDX-JSON over the artifacts.
4. **Signatures** — cosign **keyless** (Sigstore OIDC, tied to the workflow
   identity): `sign-blob` per artifact + the image digest.
5. **Image** — multi-arch (linux/amd64+arm64) pushed to ghcr.io, signed.

## 2. Verifying a release (consumers)

```bash
# checksums
sha256sum -c SHA256SUMS

# binaries + SBOM (keyless — identity comes from the certificate)
cosign verify-blob \
  --certificate flowforge-vX.Y.Z-linux-amd64.tar.gz.pem \
  --signature     flowforge-vX.Y.Z-linux-amd64.tar.gz.sig \
  --certificate-identity-regexp "https://github.com/flowforge/" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  flowforge-vX.Y.Z-linux-amd64.tar.gz

# container image
cosign verify ghcr.io/flowforge/flowforge:vX.Y.Z
```

## 3. Artifact signing (`.flow.yaml`, F-DSL-03)

Portable workflow artifacts carry their own Ed25519 detached signatures —
verifiable **offline**, independent of Sigstore:

```bash
# signer (once)
flowforge keygen ./keys              # → flowforge.key + flowforge.key.pub

# per artifact
flowforge sign invoice.flow.yaml --key keys/flowforge.key
flowforge verify invoice.flow.yaml --key keys/flowforge.key.pub
```

The `.sig` sibling file is base64 over the exact artifact bytes; any
modification fails verification (SIGN-02). Publish the public key with the
workflow (or in the release notes) so runners can verify provenance.

## 4. Local builds

```bash
scripts/build.sh vX.Y.Z      # linux/macos
scripts/build.ps1 -Version vX.Y.Z   # windows
```

Both warn when the placeholder UI would be embedded — build `app/` first for
a real release binary.

## 5. Helm

```bash
helm install flowforge chart/flowforge \
  --set image.tag=vX.Y.Z \
  --set ingress.enabled=true --set ingress.host=ff.example.com
```

Single replica by design (SQLite on a RWO PVC, `Recreate` strategy) — see
`docs/architecture.md` for the HA topology (P5).

## Checklist before tagging

- [ ] All CI jobs green on the release commit.
- [ ] `docs/progress.md` changelog updated; `CHANGELOG`-style notes drafted
      (the Release uses `generate_release_notes`).
- [ ] `server-go/ui/dist` in the repo is current (CI rebuilds it anyway).
- [ ] Post-tag: attach any out-of-band notes; announce checksums.
