#!/bin/sh
# Show the load of an NVIDIA GPU in percent, updated every second.
#   examples/gpu-load.sh
set -eu

nvidia-smi -i 0 -l 1 --query-gpu=utilization.gpu --format=csv,noheader,nounits |
	"${ATLAS:-atlas}" show
