#!/bin/bash

echo "Resetting local..."

bun run tidy
bun clean
bun install
cert-gen
env-decrypt
env-sync
sync-hosts
dev up