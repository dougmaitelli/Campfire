import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

const stylesheet = (path: string) =>
  readFileSync(new URL(`../web/src/${path}`, import.meta.url), "utf8");
const stack = stylesheet("components/forms/form-stack.css");
const controls = [
  "components/forms/fieldset.css",
  "components/forms/form-field.css",
  "components/forms/checkbox-field.css",
  "components/forms/editable-list-row.css",
  "components/servers/server-integrations.css",
  "components/servers/backup-retention.css",
  "components/embers/ember-editor.css",
  "components/permissions/permission-fields.css",
]
  .map(stylesheet)
  .join("\n");

for (const stackFirst of [true, false]) {
  test(`form containers own spacing with shared styles loaded ${stackFirst ? "first" : "last"}`, async ({
    page,
  }) => {
    await page.setContent(`<style>${stackFirst ? stack + controls : controls + stack}</style>
      <form class="form-stack" id="form">
        <div class="form-field">Name</div>
        <fieldset class="fieldset form-stack form-stack-compact permission-fields">
          <legend>Permissions</legend>
          <label class="checkbox-field"><input type="checkbox">All permissions</label>
        </fieldset>
        <fieldset class="fieldset form-stack form-stack-compact server-integration-options">
          <legend>Notifications</legend>
          <label class="checkbox-field"><input type="checkbox">Backups</label>
          <label class="checkbox-field"><input type="checkbox">Game events</label>
        </fieldset>
        <fieldset class="fieldset form-stack form-stack-compact">
          <legend>Backup retention</legend>
          <div class="backup-retention-grid">
            <div class="form-field">Copies</div><div class="form-field">Age</div>
          </div>
        </fieldset>
      </form>
      <div class="editable-list ember-variable-list form-stack form-stack-compact" id="rows">
        <fieldset class="fieldset editable-list-row">First row</fieldset>
        <fieldset class="fieldset editable-list-row">Second row</fieldset>
      </div>
      <div class="form-stack" id="ports">
        <fieldset class="fieldset form-stack"><legend>Port one</legend><div class="form-field">Interface</div><div class="form-field">Host port</div></fieldset>
        <fieldset class="fieldset form-stack"><legend>Port two</legend><div class="form-field">Interface</div><div class="form-field">Host port</div></fieldset>
      </div>`);

    for (const width of [900, 375]) {
      await page.setViewportSize({ width, height: 1000 });
      const measurements = await page
        .locator(".form-stack")
        .evaluateAll((parents) =>
          parents.map((parent) => {
            const children = Array.from(parent.children);
            const expectedGap = parseFloat(getComputedStyle(parent).rowGap);
            return {
              expectedGap,
              margins: children.map((child) => [
                getComputedStyle(child).marginTop,
                getComputedStyle(child).marginBottom,
              ]),
              gaps: children
                .slice(1)
                .map(
                  (child, index) =>
                    child.getBoundingClientRect().top -
                    children[index]!.getBoundingClientRect().bottom,
                ),
            };
          }),
        );
      for (const { expectedGap, margins, gaps } of measurements) {
        expect([12, 20]).toContain(expectedGap);
        for (const margin of margins) expect(margin).toEqual(["0px", "0px"]);
        for (const gap of gaps) expect(gap).toBeCloseTo(expectedGap);
      }
      for (const field of await page
        .locator(".backup-retention-grid > .form-field")
        .all()) {
        await expect(field).toHaveCSS("margin-bottom", "0px");
      }
    }
  });
}
