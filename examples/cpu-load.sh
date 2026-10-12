#!/bin/sh
# Show the CPU load in percent, updated every second.
#   examples/cpu-load.sh
set -eu

cpu_times() { # total and idle CPU time since boot
	read -r _ user nice system idle iowait irq softirq steal _ </proc/stat
	echo "$((user + nice + system + idle + iowait + irq + softirq + steal)) $((idle + iowait))"
}

# shellcheck disable=SC2046 # the two numbers are meant to become $1 and $2
set -- $(cpu_times)
while sleep 1; do
	total=$1 idle=$2
	set -- $(cpu_times)
	echo $((100 - 100 * ($2 - idle) / ($1 - total)))
done | "${ATLAS:-atlas}" show
