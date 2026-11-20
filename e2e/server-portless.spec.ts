import { test, expect } from "./fixtures";
import { login, openScreen } from "./screens";
import { readAuthenticatedSession } from "./auth";
import type { Server } from "../web/src/api/servers";
import type { Spark } from "../web/src/api/sparks";

for (const width of [1280, 390]) {
  test(`portless definitions create no mappings even when allocations are available (${width}px)`, async ({
    page,
  }) => {
    await login(page);
    const { csrf_token } = await readAuthenticatedSession(
      await page.request.get("/api/v1/auth/session"),
    );
    const sparks = (await (
      await page.request.get("/api/v1/sparks")
    ).json()) as Spark[];
    expect(
      sparks.some(
        (spark) => !spark.disabled && spark.port_allocations.length > 0,
      ),
    ).toBe(true);
    const imported = await page.request.post("/api/v1/embers", {
      headers: { "X-CSRF-Token": csrf_token, Origin: "http://127.0.0.1:8099" },
      data: {
        schema: 2,
        name: "Portless fixture",
        description: "No published ports",
        images: { Alpine: "alpine:3.22" },
        startup: "sleep 3600",
        work_dir: "/home/container",
        user: "1000:1000",
        variables: [],
        required_ports: [],
      },
    });
    expect(imported.ok(), await imported.text()).toBe(true);
    await page.setViewportSize({ width, height: 900 });
    await openScreen(page, "servers", "Servers");
    await page
      .getByRole("button", { name: "+ Create server", exact: true })
      .click();
    const drawer = page.getByRole("dialog", {
      name: "Create server",
      exact: true,
    });
    await drawer.getByRole("combobox", { name: "Ember", exact: true }).click();
    await page.getByRole("option", { name: /^Portless fixture ·/ }).click();
    await drawer
      .getByRole("button", { name: "Next: Resources & ports", exact: true })
      .click();
    await expect(
      drawer.getByLabel("Container port", { exact: true }),
    ).toHaveCount(0);
    for (const name of ["Next: Backups", "Next: Access", "Next: Review"]) {
      await drawer.getByRole("button", { name, exact: true }).click();
    }
    const response = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/servers") &&
        response.request().method() === "POST",
    );
    await drawer
      .getByRole("button", { name: "Create server", exact: true })
      .click();
    const saved = await response;
    expect(saved.ok(), await saved.text()).toBe(true);
    expect(saved.request().postDataJSON()).toMatchObject({ ports: [] });
    const server = (await saved.json()) as Server;
    expect(server.ports).toEqual([]);
    const persisted = await page.request.get(`/api/v1/servers/${server.id}`);
    expect(persisted.ok()).toBe(true);
    expect(await persisted.json()).toMatchObject({
      ports: [],
      game_address_port: 0,
    });
  });
}
