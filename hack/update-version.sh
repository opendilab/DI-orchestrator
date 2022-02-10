#!/bin/bash
set -eu -o pipefail

version=$1
if [[ "$version" =~ ^v ]]; then
    chart_version=${version:1}
else
    chart_version="$version"
fi

# # update chart version
# for f in "chart/Chart.yaml"; do
#     echo "update chart version to ${chart_version}"
#     sed -r "s|^(\s*)version:(\s*)(.*)|\1version: ${chart_version}|" "$f" >.tmp
#     # sed -r "s|^(\s*)version:(\s*)[0-9+][\.0-9+]*|\1version: ${chart_version}|" "$f" > .tmp
#     mv .tmp "$f"
# done

# update .gitlab-ci.yml version
for f in ".gitlab-ci.yml"; do
    echo "update ci version to ${version}"
    sed -r "s|^(\s*)VERSION:(\s*)(.*)|\1VERSION: ${version}|" "$f" >.tmp
    mv .tmp "$f"
done

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
