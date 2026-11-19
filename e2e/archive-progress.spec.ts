import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";

for (const { kind, failure } of [
  { kind: "compress", failure: false },
  { kind: "extract", failure: false },
  { kind: "compress", failure: true },
] as const) {
  test(
    failure
      ? "compression deadline failure remains visible in the dialog"
      : `${kind} progress stays inside its dialog until confirmed completion`,
    async ({ page }) => {
      await login(page);
      await openServerTab(page, "files", "Files");
      const source = kind === "compress" ? "progress.txt" : "progress.zip";
      await page.locator('input[type="file"]').setInputFiles({
        name: source,
        mimeType: "application/octet-stream",
        buffer: Buffer.from("payload"),
      });
      await expect(
        page.getByRole("checkbox", { name: `Select ${source}`, exact: true }),
      ).toBeEnabled();
      let finished = false;
      let starts = 0;
      const updatedAt = await page.evaluate(() => new Date().toISOString());
      await page.route("**/api/v1/servers/survival/files", async (route) => {
        const q = route.request().postDataJSON() as {
          operation: string;
          destination: string;
          compression?: { id: string };
          extraction?: { id: string; fresh?: boolean };
        };
        if (!q.operation.startsWith(`${kind}.`)) return route.continue();
        if (q.operation.endsWith(".start")) starts++;
        if (kind === "extract") expect(q.extraction?.fresh).toBe(true);
        const progress = {
          phase: kind === "compress" ? "archiving" : "extracting",
          input_bytes: 1024,
          output_bytes: 512,
          entries: 2,
          updated_at: updatedAt,
          ...(kind === "extract" ? { input_total: 4096 } : {}),
        };
        await route.fulfill({
          json: {
            server_id: "survival",
            path: q.destination,
            ...(kind === "compress"
              ? {
                  compression: {
                    id: q.compression!.id,
                    state: finished
                      ? failure
                        ? "failed"
                        : "completed"
                      : "running",
                    ...(finished && failure
                      ? {
                          message:
                            "Compression exceeded its time limit. Try compressing fewer files.",
                        }
                      : {}),
                    ...(!finished ? { work_progress: progress } : {}),
                  },
                }
              : {
                  extraction: {
                    id: q.extraction!.id,
                    version: 1,
                    state: finished ? "completed" : "running",
                    expires_at: new Date(Date.now() + 900000).toISOString(),
                    pending_conflicts: 0,
                    remaining_entries: finished ? 0 : 1,
                    outcomes: finished
                      ? [
                          {
                            path: source,
                            destination: "progress",
                            status: "extracted",
                          },
                        ]
                      : [],
                    ...(!finished ? { work_progress: progress } : {}),
                  },
                }),
          },
        });
      });
      if (kind === "compress") {
        await page
          .getByRole("checkbox", { name: `Select ${source}`, exact: true })
          .check();
        await page
          .getByRole("button", { name: "Archive", exact: true })
          .click();
      } else {
        await page
          .getByRole("button", { name: `Extract ${source}`, exact: true })
          .click();
      }
      const dialog = page.getByRole("dialog", {
        name: kind === "compress" ? "Archive" : "Extract archive",
        exact: true,
      });
      await dialog
        .getByRole("button", {
          name: kind === "compress" ? "Archive" : "Extract",
          exact: true,
        })
        .click();
      await expect(dialog.getByRole("progressbar")).toBeVisible();
      if (kind === "extract") await expect(dialog).toContainText("25%");
      else
        await expect(dialog.getByRole("progressbar")).not.toHaveAttribute(
          "value",
        );
      await expect(
        dialog.getByRole("button", { name: "Cancel", exact: true }),
      ).toBeEnabled();
      if (!failure) {
        await page.evaluate(() => document.fonts.ready);
        await expect(dialog).toHaveScreenshot([
          "crops",
          `${kind}-measured-progress.png`,
        ]);
        await page.setViewportSize({ width: 390, height: 844 });
        await expect(dialog).toHaveScreenshot([
          "crops",
          `${kind}-measured-progress-mobile.png`,
        ]);
      }
      finished = true;
      if (failure) {
        await expect(dialog.getByRole("alert")).toHaveText(
          "Compression exceeded its time limit. Try compressing fewer files.",
        );
        await expect(dialog.getByRole("progressbar")).toHaveCount(0);
        await expect(
          dialog.getByRole("button", { name: "Archive", exact: true }),
        ).toBeEnabled();
        await dialog
          .getByRole("button", { name: "Cancel", exact: true })
          .click();
      }
      await expect(dialog).toBeHidden();
      expect(starts).toBe(1);
    },
  );
}
