import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const [version, arch, input = "dist", output = "dist/releases"] =
  process.argv.slice(2);

if (!version || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(version)) {
  throw new Error(
    "Usage: node scripts/package-release.mjs VERSION amd64|arm64 [BINARY_DIR] [OUTPUT_DIR]",
  );
}

if (arch !== "amd64" && arch !== "arm64") {
  throw new Error("Use amd64 or arm64.");
}

const binaries = resolve(root, input);
const destination = resolve(root, output);
const stage = await mkdtemp(join(tmpdir(), "campfire-release-"));

try {
  for (const binary of ["campfire", "spark"]) {
    await cp(join(binaries, binary), join(stage, binary), {
      errorOnExist: true,
    });
  }

  await cp(join(root, "deploy"), join(stage, "deploy"), { recursive: true });
  await cp(
    join(root, "config", "campfire.env.example"),
    join(stage, "campfire.env.example"),
  );
  await cp(
    join(root, "config", "spark.env.example"),
    join(stage, "spark.env.example"),
  );
  await cp(join(root, "deploy", "INSTALL.txt"), join(stage, "INSTALL.txt"));
  await cp(join(root, "compose.yaml"), join(stage, "compose.yaml"));
  await cp(join(root, ".env.example"), join(stage, ".env.example"));

  await mkdir(destination, { recursive: true });
  const name = `campfire_${version}_linux_${arch}.tar.gz`;
  const archive = join(destination, name);

  execFileSync("tar", ["-czf", archive, "-C", stage, "."], {
    stdio: "inherit",
  });

  const digest = createHash("sha256")
    .update(await readFile(archive))
    .digest("hex");
  await writeFile(`${archive}.sha256`, `${digest}  ${name}\n`);
  console.log(`Created ${name} and its SHA-256 checksum.`);
} finally {
  await rm(stage, { recursive: true, force: true });
}
