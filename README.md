# Ubuntu Mirror Auditor

This is a Golang tool that lets you check the integrity of Ubuntu CD mirrors. It compares the contents of a mirror with the official Ubuntu archive and reports any discrepancies.

## Usage

```bash
./ubuntu-mirror-auditor --help
Ubuntu Mirror Auditor is a tool that lets you check the integrity of Ubuntu CD mirrors.
It compares the contents of a mirror with the official Ubuntu archive and reports any discrepancies.

Usage:
  ubuntu-mirror-auditor [command]

Available Commands:
  check       Check the integrity of a specific mirror
  completion  Generate the autocompletion script for the specified shell
  daemon      Run the auditor in daemon mode
  help        Help about any command
  list        List all available Ubuntu CD mirrors
  report      Generate a report of mirror checks

Flags:
  -h, --help      help for ubuntu-mirror-auditor
  -v, --verbose   Enable verbose logging

Use "ubuntu-mirror-auditor [command] --help" for more information about a command.
```

### Example

#### Check a mirror

```bash
./ubuntu-mirror-auditor check https://releases.ubuntu.com/
```

*This one should always succeed since it's the official mirror.*

#### List all mirrors

```bash
./ubuntu-mirror-auditor list
```

#### Run in daemon mode

```bash
./ubuntu-mirror-auditor list > mirrors.txt
./ubuntu-mirror-auditor daemon -m ./mirrors.txt
```

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
