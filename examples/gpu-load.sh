#!/bin/sh
# Show the load of an NVIDIA GPU in percent, updated every second.
#   examples/gpu-load.sh
set -eu

# Use atlas from your PATH, or else the one built in this repository.
atlas=${ATLAS:-$(command -v atlas || echo "$(dirname "$0")/../bin/atlas")}
command -v "$atlas" >/dev/null || { echo "atlas not found: run 'make build' first" >&2; exit 1; }

nvidia-smi -i 0 -l 1 --query-gpu=utilization.gpu --format=csv,noheader,nounits |
	"$atlas" show
