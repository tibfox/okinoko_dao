#!/usr/bin/env bash
# Builds the WASM artifacts the Go test suite embeds.
#
# Both outputs live in test/artifacts/, which is gitignored — run this once before
# `go test ./...` in a fresh clone, and again after any change under contract/ or
# mockcontract/ (the tests embed the built bytes, not the source).
#
#   main.wasm — the DAO contract itself.
#   mock.wasm — the round-12 companion contract. It is a SECOND registered contract
#               used to reach cross-contract paths a single-contract harness cannot:
#               ICC re-entrancy, ICC asset delivery, and delegation.
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p test/artifacts

build() {
	echo "building $2 -> test/artifacts/$1"
	GOTOOLCHAIN=go1.25.7 tinygo build \
		-gc=custom -scheduler=none -panic=trap -no-debug \
		-target=wasm-unknown -o "test/artifacts/$1" "./$2"
}

build main.wasm contract
build mock.wasm mockcontract

echo "done: $(ls -la test/artifacts/*.wasm | wc -l) artifact(s)"
