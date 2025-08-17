#!/bin/bash
# Auto-generated mock test for build/main.wasm (Go WASM exports)

echo "Running "projects_create"..."
wasmtime run --invoke projects_create build/main.wasm  "testitle" "testdesc" '{"testmeta": "test"}' '{"testcfg": "test"}' 1 "i64" '{"testasset": "test"}' 

echo "Running "projects_get_one"..."
wasmtime build/main.wasm --invoke "projects_get_one"

echo "Running "projects_get_all"..."
wasmtime build/main.wasm --invoke "projects_get_all"

echo "Running "projects_add_funds"..."
wasmtime build/main.wasm --invoke "projects_add_funds"

echo "Running "projects_join"..."
wasmtime build/main.wasm --invoke "projects_join"

echo "Running "projects_leave"..."
wasmtime build/main.wasm --invoke "projects_leave"

echo "Running "projects_transfer_ownership"..."
wasmtime build/main.wasm --invoke "projects_transfer_ownership"

echo "Running "projects_pause"..."
wasmtime build/main.wasm --invoke "projects_pause"

echo "Running "proposals_create"..."
wasmtime build/main.wasm --invoke "proposals_create"

echo "Running "proposals_vote"..."
wasmtime build/main.wasm --invoke "proposals_vote"

echo "Running "proposals_tally"..."
wasmtime build/main.wasm --invoke "proposals_tally"

echo "Running "proposals_execute"..."
wasmtime build/main.wasm --invoke "proposals_execute"

echo "Running "proposals_get_one"..."
wasmtime build/main.wasm --invoke "proposals_get_one"

echo "Running "proposals_get_all"..."
wasmtime build/main.wasm --invoke "proposals_get_all"

