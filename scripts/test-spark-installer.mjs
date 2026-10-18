import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";
import {
  sparkInstallCommand,
  validAgentAddress,
} from "../web/src/components/sparks/installCommand.ts";

const installer = resolve("web/public/install-spark.sh");

test("installer lists every missing dependency before changing the host", () => {
  const result = spawnSync(
    "/bin/bash",
    [installer, "--panel", "https://panel.test:8443", "--token", "testtoken"],
    { env: { PATH: "" }, encoding: "utf8" },
  );
  assert.equal(result.status, 1);
  assert.match(result.stderr, /Missing required dependencies:/);
  for (const name of [
    "curl",
    "tar",
    "sha256sum",
    "install",
    "systemctl",
    "docker",
    "uname",
    "mktemp",
    "chmod",
    "rm",
    "grep",
  ]) {
    assert.ok(
      result.stderr.includes(`  - ${name}\n`),
      `missing ${name} from report`,
    );
  }
});

test("installer command preserves arguments without executing shell syntax", async () => {
  const directory = await mkdtemp(join(tmpdir(), "spark-installer-test-"));
  try {
    const log = join(directory, "arguments");
    const sentinel = join(directory, "injected");
    await writeFile(
      join(directory, "curl"),
      '#!/bin/bash\nwhile (($#)); do\n if [[ $1 == --output ]]; then cp "$MOCK_INSTALLER" "$2"; exit; fi\n shift\ndone\nexit 1\n',
      { mode: 0o755 },
    );
    await writeFile(join(directory, "sudo"), '#!/bin/bash\nexec "$@"\n', {
      mode: 0o755,
    });
    const mockInstaller = join(directory, "installer.sh");
    await writeFile(
      mockInstaller,
      '#!/bin/bash\nprintf "%s\\0" "$@" > "$MOCK_LOG"\n',
    );
    const ca = `/private/CA's file $(touch ${sentinel}); certificate.crt`;
    const command = sparkInstallCommand(
      "https://campfire.test",
      "https://agents.test:8443",
      "testtoken",
      ca,
    );
    const result = spawnSync("/bin/bash", ["-c", command], {
      env: {
        PATH: `${directory}:/usr/bin:/bin`,
        MOCK_LOG: log,
        MOCK_INSTALLER: mockInstaller,
      },
      encoding: "utf8",
    });
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual((await readFile(log, "utf8")).split("\0").slice(0, -1), [
      "--panel",
      "https://agents.test:8443",
      "--token",
      "testtoken",
      "--ca-file",
      ca,
    ]);
    await assert.rejects(readFile(sentinel), { code: "ENOENT" });
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("agent address requires an HTTPS origin", () => {
  assert.equal(validAgentAddress("https://agents.test:8443"), true);
  for (const value of [
    "http://agents.test",
    "https://user:pass@agents.test",
    "https://agents.test/spark",
    "https://agents.test?token=secret",
    "https://agents.test#fragment",
    "invalid",
  ]) {
    assert.equal(validAgentAddress(value), false);
    assert.equal(
      sparkInstallCommand("https://campfire.test", value, "token", ""),
      "",
    );
  }
});
