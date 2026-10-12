#!/bin/sh
# Show the ping time to a host in milliseconds, with one decimal below 100 ms.
#   examples/ping.sh              pings 1.1.1.1
#   examples/ping.sh example.com
set -eu

# Use atlas from your PATH, or else the one built in this repository.
atlas=${ATLAS:-$(command -v atlas || echo "$(dirname "$0")/../bin/atlas")}
command -v "$atlas" >/dev/null || { echo "atlas not found: run 'make build' first" >&2; exit 1; }

ping -n "${1:-1.1.1.1}" | awk -F'time=' '/time=/ {
	ms = $2 + 0
	if (ms >= 999) print 999 # the display has three digits
	else if (ms >= 99.95) printf "%d\n", ms
	else printf "%.1f\n", ms
	fflush()
}' | "$atlas" show
