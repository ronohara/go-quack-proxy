# quack-proxy Flag Ordering Problem

## Issue Description

When running quack-proxy with flags, the order of arguments matters. The `flag` package in Go stops parsing at the first non-flag argument. Since `quack-proxy` uses subcommands (`start`, `stop`, `status`, etc.), the subcommand acts as a non-flag argument that stops flag parsing.

## Example

### Does NOT Work (flags after subcommand)

quack-proxy start -c config.yaml --verbose --log-file quack.log

### Works (flags before subcommand)

quack-proxy --verbose --log-file quack.log start -c config.yaml

### Works (config flag before subcommand, others after)

quack-proxy -c config.yaml --verbose start

## Why This Happens

In Go's `flag` package:
- `flag.Parse()` parses all command-line arguments
- It stops at the first non-flag argument (an argument that doesn't start with `-`)
- The subcommand (`start`, `stop`, etc.) is the first non-flag argument
- Any flags after the subcommand are ignored

## Solution

Always place flags before the subcommand:

quack-proxy [flags] <subcommand> [subcommand-flags]

Or use the `-c` config flag before the subcommand:

quack-proxy -c config.yaml <subcommand>

## Recommended Usage

# Start with verbose logging and log file
quack-proxy --verbose --log-file /var/log/quack.log start -c config.yaml

# Start with debug logging (includes SQL)
quack-proxy --debug start -c config.yaml

# Status with JSON output
quack-proxy status --json

# Stop the service
quack-proxy stop

## Documentation Notes

This should be documented in:
1. `README.md` - in the usage examples
2. `docs/PRD.md` - in the CLI interface section
3. The `--help` output should show examples with flags before subcommands

## Future Improvement

Consider using `flag.NewFlagSet` for each subcommand to allow flags after the subcommand. This would make the CLI more intuitive and consistent with standard CLI patterns.