#!/bin/sh
# Show how much memory is in use, in percent with one decimal, every 2 seconds.
#   examples/memory.sh
set -eu

while :; do
	awk '/^MemTotal:/ { total = $2 }
	     /^MemAvailable:/ { free = $2 }
	     END {
	         used = 100 * (total - free) / total
	         if (used > 99.9) used = 99.9 # the display has three digits
	         printf "%.1f\n", used
	     }' /proc/meminfo
	sleep 2
done | "${ATLAS:-atlas}" show
