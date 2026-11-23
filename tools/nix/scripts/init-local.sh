#!/bin/bash

echo "Initializing local..."
bun run tidy
bun install
cert-gen
env-decrypt
env-sync
sync-hosts
dev up