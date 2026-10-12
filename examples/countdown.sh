#!/bin/sh
# Count down from a number of seconds to 000, which then stays until Ctrl-C.
#   examples/countdown.sh 90
set -eu

# Use atlas from your PATH, or else the one built in this repository.
atlas=${ATLAS:-$(command -v atlas || echo "$(dirname "$0")/../bin/atlas")}
command -v "$atlas" >/dev/null || { echo "atlas not found: run 'make build' first" >&2; exit 1; }

n=${1:?usage: countdown.sh SECONDS}
if ! [ "$n" -ge 0 ] 2>/dev/null || [ "$n" -gt 999 ]; then
	echo "countdown.sh: give a number of seconds from 0 to 999" >&2
	exit 1
fi

while [ "$n" -ge 0 ]; do
	echo "$n"
	n=$((n - 1))
	sleep 1
done | "$atlas" show
