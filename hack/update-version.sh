#!/bin/bash
set -eu -o pipefail

version=$1

# update chart version
for f in "./.github/workflows/release.yaml"; do
    echo "update release version to ${version}"
    sed -r "s|^(\s*)version:(\s*)(.*)|\1version: ${version}|" "$f" >.tmp
    mv .tmp "$f"
done
