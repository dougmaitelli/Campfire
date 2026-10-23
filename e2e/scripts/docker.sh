#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"
playwright_version=$(node --input-type=commonjs -e 'const v = require("./e2e/package.json").devDependencies["@playwright/test"]; if (!/^\d+\.\d+\.\d+$/.test(v)) throw new Error("Pin an exact Playwright version"); process.stdout.write(v)')
docker build --build-arg "PLAYWRIGHT_VERSION=$playwright_version" -f e2e/Dockerfile -t campfire-e2e e2e
exec docker run --rm --ipc=host \
  -v "$root:/work" \
  -v /work/e2e/node_modules \
  -v /work/web/node_modules \
  -v /work/web/dist \
  -v /work/.cache \
  campfire-e2e sh -c 'pnpm install --frozen-lockfile && pnpm --dir ../web install --frozen-lockfile && pnpm run typecheck:e2e && pnpm run test:e2e "$@"' sh "$@"
