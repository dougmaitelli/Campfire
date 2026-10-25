import { expect, test } from "vitest";
import ts from "typescript";
import i18n from "../../src/i18n";
import en from "../../src/i18n/locales/en.json";

test("English fallback supports interpolation and plural forms", async () => {
  await i18n.changeLanguage("unsupported");
  expect(i18n.t("common.cancel")).toBe("Cancel");
  for (const count of [0, 1, 2])
    expect(i18n.t("sparksPage.count", { count })).toBe(
      `${count} Spark${count === 1 ? "" : "s"}`,
    );
  expect(i18n.t("users.displayLabel", { name: "Alex", username: "alex" })).toBe(
    "Alex (alex)",
  );
});

test("visible JSX text and accessibility labels belong in translation catalogs", () => {
  const attributes = new Set([
    "label",
    "title",
    "description",
    "placeholder",
    "aria-label",
    "aria-description",
    "aria-roledescription",
    "alt",
    "emptyMessage",
    "emptyDescription",
    "loadingLabel",
    "editLabel",
    "notice",
    "resource",
  ]);
  const sources = import.meta.glob<string>("/src/**/*.{ts,tsx}", {
    query: "?raw",
    import: "default",
    eager: true,
  });
  const failures: string[] = [];
  for (const [path, input] of Object.entries(sources)) {
    const parsed = ts.createSourceFile(
      path,
      input,
      ts.ScriptTarget.Latest,
      true,
      path.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    );
    function visit(node: ts.Node) {
      const visible = ts.isJsxText(node)
        ? node.text
        : ts.isJsxAttribute(node) &&
            attributes.has(node.name.getText(parsed)) &&
            node.initializer &&
            ts.isStringLiteral(node.initializer)
          ? node.initializer.text
          : "";
      if (/[A-Za-z]/.test(visible))
        failures.push(
          `${path}:${parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line + 1}: ${visible.trim()}`,
        );
      ts.forEachChild(node, visit);
    }
    visit(parsed);
  }
  expect(failures).toEqual([]);
  expect(Object.keys(en).length).toBeGreaterThan(10);
});
