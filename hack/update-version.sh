#!/bin/bash
set -eu -o pipefail

version=$1
app_version=$2
if [[ "$version" =~ ^v ]]; then
    chart_version=${version:1}
else
    chart_version="$version"
fi

# update chart version
for f in "chart/Chart.yaml"; do
    echo "update chart version to ${chart_version}"
    sed -r "s|^(\s*)version:(\s*)(.*)|\1version: ${chart_version}|" "$f" >.tmp
    mv .tmp "$f"
done

# update chart app version
for f in "chart/Chart.yaml"; do
    echo "update chart app version to ${app_version}"
    sed -r "s|^(\s*)appVersion:(\s*)(.*)|\1appVersion: ${app_version}|" "$f" >.tmp
    mv .tmp "$f"
done

# update chart value tag
for f in "chart/values.yaml"; do
    echo "update chart value tag to ${version}"
    sed -r "s|^(\s*)tag:(\s*)(.*)|\1tag: ${version}|" "$f" >.tmp
    mv .tmp "$f"
done

for f in ".github/workflows/release.yaml"; do
    echo "update github action version to ${version}"
    sed -r "s|^(\s*)version:(\s*)(.*)|\1version: ${version}|" "$f" >.tmp
    mv .tmp "$f"
done
