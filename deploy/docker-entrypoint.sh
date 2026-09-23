#!/bin/sh
set -eu

# The store is in-memory, so an ephemeral key is safe for local Compose runs:
# a container restart loses both the key and the encrypted records. Production
# deployments should provide a stable PII_AES_KEY through a secret manager.
if [ -z "${PII_AES_KEY:-}" ]; then
    PII_AES_KEY="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
    export PII_AES_KEY
    echo "PII_AES_KEY is not set; generated an ephemeral key for this container" >&2
fi

exec /usr/local/bin/server "$@"
