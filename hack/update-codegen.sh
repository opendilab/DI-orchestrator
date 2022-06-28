#!/usr/bin/env bash

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
MODULE="opendilab.org/di-orchestrator"

deepcopy-gen --bounding-dirs "${MODULE}/pkg/common/types" \
    --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    --input-dirs "${MODULE}/pkg/common/types" \
    --output-base "${SCRIPT_ROOT}" \
    --output-file-base "zz_generated.deepcopy"

mv ${MODULE}/pkg/common/types/* ${SCRIPT_ROOT}/pkg/common/types/
rm -rf $(dirname ${MODULE})
