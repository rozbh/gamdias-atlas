#!/bin/sh
# A clock. The dot blinks once a second, like the colon on a digital clock.
# The display has three digits, so it shows the last three of the 12-hour
# time: 9:41 is 941, 10:05 is 005 and 12:30 is 230.
#   examples/clock.sh
set -eu

# Use atlas from your PATH, or else the one built in this repository.
atlas=${ATLAS:-$(command -v atlas || echo "$(dirname "$0")/../bin/atlas")}
command -v "$atlas" >/dev/null || { echo "atlas not found: run 'make build' first" >&2; exit 1; }

while :; do
	# shellcheck disable=SC2046 # the two fields are meant to become $1 and $2
	set -- $(date '+%I%M %s') # for example: 0941 1791850000
	digits=${1#?}             # 941
	if [ $(($2 % 2)) -eq 0 ]; then
		echo "${digits%?}.${digits#??}" # 94.1
	else
		echo "$digits"
	fi
	sleep 0.1
done | "$atlas" show
