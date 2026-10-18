#!/usr/bin/env bash
# Build both Linux agents before embedding their packages in Campfire.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
destination=$root/backend/internal/campfire/sparkbundle/dist
stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$destination" "$stage/deploy/systemd"
cp "$root/deploy/systemd/spark.service" "$stage/deploy/systemd/spark.service"
flags="-s -w -X campfire/internal/shared/buildinfo.Version=${VERSION:-dev} -X campfire/internal/shared/buildinfo.Commit=${COMMIT:-unknown} -X campfire/internal/shared/buildinfo.Date=${BUILD_DATE:-unknown}"
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch go -C "$root/backend" build -buildvcs=false -trimpath -ldflags "$flags" -o "$stage/spark" ./cmd/spark
  archive=spark_linux_$arch.tar.gz
  tar -czf "$destination/$archive" -C "$stage" ./spark ./deploy/systemd/spark.service
  (cd "$destination" && sha256sum "$archive" > "$archive.sha256")
done
