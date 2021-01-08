#!/bin/bash

CUR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
pushd "$CUR_DIR" || exit

pip3 install -r requirements.txt

export OPERATOR_NAMESPACE=test
python3 -m test --only=operator/*

popd || exit