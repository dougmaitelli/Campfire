import { readFile, access } from "node:fs/promises";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const require = createRequire(
  new URL("../../web/package.json", import.meta.url),
);
const ts = require("typescript");
const modules = new Map();

async function moduleURL(url) {
  if (modules.has(url.href)) return modules.get(url.href);
  const pending = (async () => {
    const input = await readFile(url, "utf8");
    const source = ts.transpileModule(input, {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
        jsx: ts.JsxEmit.ReactJSX,
      },
      fileName: url.pathname,
    }).outputText;
    const parsed = ts.createSourceFile(
      url.pathname,
      source,
      ts.ScriptTarget.Latest,
      true,
    );
    const edits = [];
    for (const statement of parsed.statements) {
      if (!ts.isImportDeclaration(statement)) continue;
      const specifier = statement.moduleSpecifier.text;
      let target;
      if (specifier.endsWith(".css")) target = "data:text/javascript,export{}";
      else if (specifier.startsWith(".")) {
        const dependency = new URL(specifier, url);
        if (dependency.pathname.endsWith("/i18n"))
          target = new URL("./i18n-instance.mjs", import.meta.url).href;
        else {
          if (!/\.(?:tsx?|mjs)$/.test(dependency.pathname)) {
            dependency.pathname += ".ts";
            try {
              await access(dependency);
            } catch {
              dependency.pathname += "x";
            }
          }
          target = await moduleURL(dependency);
        }
      } else target = pathToFileURL(require.resolve(specifier)).href;
      edits.push({
        start: statement.moduleSpecifier.getStart(parsed),
        end: statement.moduleSpecifier.end,
        value: JSON.stringify(target),
      });
    }
    let output = source;
    for (const edit of edits.sort((a, b) => b.start - a.start))
      output =
        output.slice(0, edit.start) + edit.value + output.slice(edit.end);
    return `data:text/javascript;base64,${Buffer.from(output).toString("base64")}`;
  })();
  modules.set(url.href, pending);
  return pending;
}

export async function loadWebModule(url) {
  return import(await moduleURL(url));
}
