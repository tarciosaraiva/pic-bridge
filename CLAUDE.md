# CLAUDE.md

Go CLI (`picb`) that imports photos/videos from camera SD cards. See README.md
and `docs/superpowers/specs/` for the design.

## Rules

- Never overwrite a file in the destination. Copy to a temp name, verify
  SHA-256, then rename.
- A file is "imported" only after its copy is verified and its manifest entry
  is written.
- Bottleneck is card I/O, not CPU: keep the copy worker pool small.
- Deferred work lives as GitHub issues labelled `later`, ranked
  `priority-zero` / `priority-one` / `priority-two`. Version 1 shipped as
  v1.0.0; pick `later` issues up in priority order.
- Releases: release-please keeps a release PR open on `main`; merging it tags
  `vX.Y.Z` and attaches darwin tarballs. Commit prefixes set the bump
  (`fix:` patch, `feat:` minor).
- Prefer the Go standard library. Current deps: imagemeta (EXIF),
  go.yaml.in/yaml/v3 (config + manifest).
