# Ubuntu Mirror Auditor

This is a Golang tool that lets you check the integrity of Ubuntu CD mirrors. It compares the contents of a mirror with the official Ubuntu archive and reports any discrepancies.

## Installation

### Prerequisites

- Go 1.21 or later
- GCC (required for SQLite3 CGO bindings)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/gauthier/ubuntu-mirror-auditor
cd ubuntu-mirror-auditor

# Build the binary
make build

# Or use go build directly
go build -o ubuntu-mirror-auditor ./cmd/ubuntu-mirror-auditor

# Install to /usr/local/bin (optional)
sudo make install
```

## Copyright and License

```txt
ubuntu-mirror-auditor  Copyright (C) 2026  Gauthier Jolly
This program comes with ABSOLUTELY NO WARRANTY; for details type `show w'.
This is free software, and you are welcome to redistribute it
under certain conditions; type `show c' for details.
```
