#!/bin/sh
# Show the ping time to a host in milliseconds, with one decimal below 100 ms.
#   examples/ping.sh              pings 1.1.1.1
#   examples/ping.sh example.com
set -eu

ping -n "${1:-1.1.1.1}" | awk -F'time=' '/time=/ {
	ms = $2 + 0
	if (ms >= 999) print 999 # the display has three digits
	else if (ms >= 99.95) printf "%d\n", ms
	else printf "%.1f\n", ms
	fflush()
}' | "${ATLAS:-atlas}" show
