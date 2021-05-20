#!/bin/bash
set -eu -o pipefail

dir=$1
image_tag=$2

find "$dir" -type f -name '*.yaml' | while read -r f; do
    echo "$f"
    sed "s|registry.sensetime.com/\(.*\):.*|registry.sensetime.com/\1:${image_tag}|" "$f" >.tmp
    mv .tmp "$f"
done