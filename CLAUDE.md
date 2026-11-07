# CLAUDE.md

Go CLI (`picb`) that imports photos/videos from camera SD cards. See README.md
and `docs/superpowers/specs/` for the design.

## Rules

- Never overwrite a file in the destination. Copy to a temp name, verify
  SHA-256, then rename.
- A file is "imported" only after its copy is verified and its manifest entry
  is written.
- Bottleneck is card I/O, not CPU: keep the copy worker pool small.
- Deferred features live as GitHub issues labelled `later`.
  Don't build them into version 1.
- Prefer the Go standard library. Current deps: imagemeta (EXIF),
  go.yaml.in/yaml/v3 (config + manifest).
