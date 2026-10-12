#!/bin/sh
# A clock for three digits.
#
#   examples/clock.sh      12-hour. From 1:00 to 9:59 the time fits: 9:41 is
#                          941, and the dot blinks once a second like a colon.
#                          From 10:00 to 12:59 it has a digit too many, so the
#                          display takes turns, two seconds each: the hour with
#                          the dot on (12.0), then the minutes with it off (035).
#   examples/clock.sh 24   24-hour, always taking turns: 14.0, then 035.
#
# The display has no blank digit, so the place that is not used shows a 0.
set -eu

# Use atlas from your PATH, or else the one built in this repository.
atlas=${ATLAS:-$(command -v atlas || echo "$(dirname "$0")/../bin/atlas")}
command -v "$atlas" >/dev/null || { echo "atlas not found: run 'make build' first" >&2; exit 1; }

format=${1:-12}
case $format in
12 | 24) ;;
*)
	echo "usage: clock.sh [12|24]" >&2
	exit 1
	;;
esac

while :; do
	# shellcheck disable=SC2046 # the fields are meant to become $1 to $4
	set -- $(date '+%I %H %M %s') # for example: 09 21 41 1791850000
	hour=$1 minute=$3 second=$4
	[ "$format" = 24 ] && hour=$2

	if [ "$format" = 12 ] && [ "${hour#0}" -lt 10 ]; then
		if [ $((second % 2)) -eq 0 ]; then
			echo "${hour#0}${minute%?}.${minute#?}" # 94.1
		else
			echo "${hour#0}$minute" # 941
		fi
	elif [ $((second % 4)) -lt 2 ]; then
		echo "$hour.0" # 12.0: the dot marks the hour
	else
		echo "0$minute" # 035
	fi
	sleep 0.1
done | "$atlas" show
