#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

if [[ -f "$PROJECT_ROOT/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    . "$PROJECT_ROOT/.env"
    set +a
fi

: "${AWS_ACCESS_KEY_ID:?must be set}"
: "${AWS_SECRET_ACCESS_KEY:?must be set}"
: "${S3_BUCKET:?must be set}"
: "${S3_ENDPOINT:?must be set}"

aws s3api put-bucket-lifecycle-configuration \
    --endpoint-url "$S3_ENDPOINT" \
    --bucket "$S3_BUCKET" \
    --lifecycle-configuration "file://$SCRIPT_DIR/lifecycle.json"

echo "applied lifecycle config to s3://$S3_BUCKET"
