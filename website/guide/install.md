# Installation

`picb` runs on macOS. Card cleaning also works on Linux.

## Release tarball

Download the tarball for your Mac from the
[latest release](https://github.com/tarciosaraiva/pic-bridge/releases/latest):

- `arm64` for Apple Silicon
- `amd64` for Intel

Extract it and put `picb` somewhere on your `PATH`.

## With Go

Needs Go 1.27+.

```sh
go install github.com/tarciosaraiva/pic-bridge/cmd/picb@latest
```

## Optional: rclone

[Archiving a folder](./usage#archiving-a-folder) needs
[rclone](https://rclone.org):

```sh
brew install rclone
```

Next: [Configuration](./configuration).
