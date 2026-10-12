#!/bin/sh
# Show how much memory is in use, in percent with one decimal, every 2 seconds.
#   examples/memory.sh
set -eu

# Use atlas from your PATH, or else the one built in this repository.
atlas=${ATLAS:-$(command -v atlas || echo "$(dirname "$0")/../bin/atlas")}
command -v "$atlas" >/dev/null || { echo "atlas not found: run 'make build' first" >&2; exit 1; }

while :; do
	awk '/^MemTotal:/ { total = $2 }
	     /^MemAvailable:/ { free = $2 }
	     END {
	         used = 100 * (total - free) / total
	         if (used > 99.9) used = 99.9 # the display has three digits
	         printf "%.1f\n", used
	     }' /proc/meminfo
	sleep 2
done | "$atlas" show
