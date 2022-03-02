#!/bin/bash
set -eu -o pipefail

version=$1

for f in "config/manager/di_config.yaml"; do
    echo "update config map orchestrator version to ${version}"
    sed -r "s|^(\s*)DI_ORCHESTRATOR_VERSION:(\s*)(.*)|\1DI_ORCHESTRATOR_VERSION: ${version}|" "$f" >.tmp
    mv .tmp "$f"
done

for f in ".github/workflows/release.yaml"; do
    echo "update github action version to ${version}"
    sed -r "s|^(\s*)version:(\s*)(.*)|\1version: ${version}|" "$f" >.tmp
    mv .tmp "$f"
done
