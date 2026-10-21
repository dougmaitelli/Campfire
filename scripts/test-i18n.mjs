import assert from "node:assert/strict";
import { readFile, readdir } from "node:fs/promises";
import { createRequire } from "node:module";
import test from "node:test";
import i18n from "./helpers/i18n-instance.mjs";
import { loadWebModule } from "./helpers/load-web-module.mjs";

const require = createRequire(new URL("../web/package.json", import.meta.url));
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const ts = require("typescript");
const root = new URL("../web/src/", import.meta.url);
const en = JSON.parse(
  await readFile(new URL("i18n/locales/en.json", root), "utf8"),
);

test("network groups keep addresses outside pills and preserve port mappings", async () => {
  const { PortGroups } = await loadWebModule(
    new URL("components/network/PortGroups.tsx", root),
  );
  const markup = renderToStaticMarkup(
    React.createElement(PortGroups, {
      ports: [
        { address: "::1", port: 2456, protocol: "udp", targetPort: 3000 },
        { address: "::1", port: "2457–2458", protocol: "udp" },
        { address: "127.0.0.1", port: 8080, protocol: "tcp" },
      ],
    }),
  );
  assert.equal((markup.match(/class="port-group-address"/g) ?? []).length, 2);
  assert.equal((markup.match(/::1/g) ?? []).length, 1);
  assert.match(markup, /2456 UDP/);
  assert.match(markup, /2457–2458 UDP/);
  assert.match(markup, /3000 in container/);
  assert.ok(!markup.includes("::1:2456"));
  assert.equal(
    renderToStaticMarkup(React.createElement(PortGroups, { ports: [] })),
    "-",
  );
});

test("English fallback, interpolation, and plural forms", async () => {
  await i18n.changeLanguage("unsupported");
  assert.equal(i18n.t("common.cancel"), "Cancel");
  assert.equal(i18n.t("sparksPage.count", { count: 1 }), "1 Spark");
  assert.equal(i18n.t("sparksPage.count", { count: 0 }), "0 Sparks");
  assert.equal(i18n.t("sparksPage.count", { count: 2 }), "2 Sparks");
  assert.equal(
    i18n.t("users.displayLabel", { name: "Alex", username: "alex" }),
    "Alex (alex)",
  );
  await i18n.changeLanguage("en");
});

test("resource columns display CPU percentages and preserve allocation ratios", async () => {
  const { ResourceReadings } = await loadWebModule(
    new URL("components/servers/ResourceReadings.tsx", root),
  );
  const markup = renderToStaticMarkup(
    React.createElement(
      "table",
      null,
      React.createElement(
        "tbody",
        null,
        React.createElement(
          "tr",
          null,
          React.createElement(ResourceReadings, {
            usage: {
              cpu_percent: 150,
              memory_bytes: 1048576,
              disk_bytes: 2097152,
              sampled_at: 100,
              disk_sampled_at: 100,
              sample_age_seconds: 0,
              disk_age_seconds: 0,
              container_error: "",
              disk_error: "",
            },
            cpuLimit: 2,
            memoryMB: 4,
            diskMB: 8,
            elapsed: 0,
            layout: "columns",
          }),
        ),
      ),
    ),
  );
  const cells = [...markup.matchAll(/<td>([\s\S]*?)<\/td>/g)].map(
    (match) => match[1],
  );
  assert.equal(cells.length, 3);
  assert.match(cells[0], /75%/);
  assert.match(cells[0], /value="0.75"/);
  assert.ok(!cells[0].includes("usage-allocation"));
  assert.match(cells[1], /1 MiB/);
  assert.match(cells[1], /4 MiB/);
  assert.ok(cells.every((cell) => cell.includes("usage-reading")));
  assert.ok(cells.every((cell) => cell.includes("visually-hidden")));
});

test("Overview setup marks existing Sparks complete and respects management access", async () => {
  const { OverviewEmptyState } = await loadWebModule(
    new URL("components/overview/OverviewEmptyState.tsx", root),
  );
  const render = (sparkCount, permissions = {}) =>
    renderToStaticMarkup(
      React.createElement(OverviewEmptyState, {
        sparkCount,
        canManageSparks: true,
        canManageEmbers: true,
        canCreateServers: true,
        ...permissions,
      }),
    );
  const fresh = render(0);
  assert.match(fresh, /Start your campfire/);
  assert.match(fresh, /Add Spark/);
  assert.match(fresh, /href="#embers"/);
  assert.ok(!fresh.includes("overview-setup-complete"));
  const ready = render(1);
  assert.match(ready, /overview-setup-complete/);
  assert.match(ready, /Complete/);
  assert.match(ready, /View Sparks/);
  assert.ok(!ready.includes("Add Spark"));
  const restricted = render(undefined, {
    canManageSparks: false,
    canManageEmbers: false,
    canCreateServers: false,
  });
  assert.match(restricted, /Ask an administrator/);
  assert.ok(!restricted.includes('href="#sparks"'));
  assert.ok(!restricted.includes('href="#embers"'));
});

test("warning confirmations use the primary action and lock all dismissal controls while busy", async () => {
  const { ConfirmDialog } = await loadWebModule(
    new URL("components/layout/ConfirmDialog.tsx", root),
  );
  const render = (overrides = {}) =>
    renderToStaticMarkup(
      React.createElement(ConfirmDialog, {
        title: "Reinstall server",
        description: "Choose how to reinstall this server.",
        confirmLabel: "Reinstall",
        warning: true,
        busy: false,
        error: null,
        onClose: () => {},
        onConfirm: () => {},
        ...overrides,
      }),
    );
  assert.match(render(), /button-primary/);
  assert.ok(!render().includes("button-warning"));
  assert.match(render({ danger: true }), /button-danger/);
  const busy = render({ busy: true });
  const buttons = [...busy.matchAll(/<button\b[^>]*>/g)];
  assert.equal(buttons.length, 3);
  assert.ok(buttons.every(([button]) => button.includes("disabled=")));
});

test("radio fields retain native grouping, selected state, and descriptive labels", async () => {
  const { RadioField } = await loadWebModule(
    new URL("components/forms/RadioField.tsx", root),
  );
  const markup = renderToStaticMarkup(
    React.createElement(RadioField, {
      name: "install-mode",
      value: "keep",
      checked: true,
      disabled: true,
      onChange: () => {},
      description: "Preserve game files.",
      children: "Keep existing files",
    }),
  );
  assert.match(markup, /type="radio"/);
  assert.match(markup, /name="install-mode"/);
  assert.match(markup, /checked=""/);
  assert.match(markup, /disabled=""/);
  assert.match(markup, /Keep existing files/);
  assert.match(markup, /Preserve game files/);
});

test("server status timing uses real start and stop timestamps", async () => {
  const { ServerState } = await loadWebModule(
    new URL("components/servers/ServerState.tsx", root),
  );
  const render = (props) =>
    renderToStaticMarkup(
      React.createElement(ServerState, { now: 200000, ...props }),
    );
  assert.match(render({ state: "running", startedAt: 192740 }), /Uptime 2h 1m/);
  assert.match(
    render({ state: "stopped", stoppedAt: 199400 }),
    /Stopped 10m ago/,
  );
  assert.ok(!render({ state: "running" }).includes("server-state-timing"));
  assert.ok(
    !render({ state: "unknown", stoppedAt: 199400 }).includes(
      "server-state-timing",
    ),
  );
  assert.ok(
    !render({ state: "running", startedAt: 300000 }).includes(
      "server-state-timing",
    ),
  );
});

test("dashboard quick controls respect server state and permissions", async () => {
  const { ServerQuickControls } = await loadWebModule(
    new URL("components/servers/ServerQuickControls.tsx", root),
  );
  for (const [
    state,
    busy,
    operation,
    canOperate,
    startDisabled,
    stopDisabled,
  ] of [
    ["running", false, undefined, true, true, false],
    ["stopped", false, undefined, true, false, true],
    ["restarting", false, undefined, true, true, false],
    ["installing", true, "install", true, true, false],
    ["stopped", false, undefined, false, true, true],
    ["not_installed", false, undefined, true, true, true],
    ["unknown", false, undefined, true, true, true],
  ]) {
    const markup = renderToStaticMarkup(
      React.createElement(ServerQuickControls, {
        id: "server/one",
        name: "Test world",
        status: { state, busy, operation },
        disabled: false,
        canOperate,
        onStatus: () => {},
        onError: () => {},
      }),
    );
    const buttons = [...markup.matchAll(/<button\b[^>]*>/g)].map(
      (match) => match[0],
    );
    assert.equal(buttons.length, 2);
    assert.equal(
      buttons[0].includes("disabled="),
      startDisabled,
      `${state}: start`,
    );
    assert.equal(
      buttons[1].includes("disabled="),
      stopDisabled,
      `${state}: stop`,
    );
  }
});

test("UI components render the selected language without changing protocol values", async () => {
  const { ServerLifecycleControls } = await loadWebModule(
    new URL("components/servers/ServerLifecycleControls.tsx", root),
  );
  const props = {
    status: { state: "not_installed", busy: false },
    disabled: false,
    canConfigure: true,
    onAction: () => {},
  };
  assert.match(
    renderToStaticMarkup(React.createElement(ServerLifecycleControls, props)),
    />Install</,
  );
  i18n.addResourceBundle(
    "test",
    "translation",
    { common: { install: "Installer", start: "Démarrer" } },
    true,
    true,
  );
  await i18n.changeLanguage("test");
  const output = renderToStaticMarkup(
    React.createElement(ServerLifecycleControls, props),
  );
  assert.match(output, />Installer</);
  assert.match(output, />Démarrer</);
  assert.equal(props.status.state, "not_installed");
  await i18n.changeLanguage("en");
});

test("installer controls allow stop and kill while keeping other mutations disabled", async () => {
  const { ServerLifecycleControls } = await loadWebModule(
    new URL("components/servers/ServerLifecycleControls.tsx", root),
  );
  for (const operation of ["install", "reinstall", "update"]) {
    const output = renderToStaticMarkup(
      React.createElement(ServerLifecycleControls, {
        status: { state: "installing", operation, busy: true },
        disabled: false,
        canConfigure: true,
        onAction: () => {},
      }),
    );
    const buttons = [...output.matchAll(/<button\b([^>]*)>(.*?)<\/button>/g)];
    for (const [, attributes, label] of buttons) {
      assert.equal(
        attributes.includes("disabled"),
        !["Stop", "Kill"].includes(label),
      );
    }
    assert.ok(buttons.some(([, , label]) => label === "Stop"));
    assert.ok(buttons.some(([, , label]) => label === "Kill"));
  }
});

test("translated interpolation remains escaped by React", () => {
  const value = i18n.t("users.displayLabel", {
    name: '<img src=x onerror="alert(1)">',
    username: "alex",
  });
  const output = renderToStaticMarkup(React.createElement("span", null, value));
  assert.ok(!output.includes("<img"));
  assert.ok(output.includes("&lt;img"));
});

test("rich-text catalog messages preserve links and text in translated order", () => {
  const { Trans } = require("react-i18next");
  i18n.addResourceBundle(
    "test",
    "translation",
    { test: { linked: "<reference/> — details for <name/>" } },
    true,
    true,
  );
  const output = renderToStaticMarkup(
    React.createElement(Trans, {
      i18n,
      t: i18n.getFixedT("test"),
      i18nKey: "test.linked",
      components: {
        reference: React.createElement("a", { href: "#sparks/node" }, "Node"),
        name: React.createElement(React.Fragment, null, "<name>"),
      },
    }),
  );
  assert.match(
    output,
    /href="#sparks\/node">Node<\/a> — details for &lt;name&gt;/,
  );
});

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  return (
    await Promise.all(
      entries.map((entry) => {
        const url = new URL(
          entry.name + (entry.isDirectory() ? "/" : ""),
          directory,
        );
        return entry.isDirectory()
          ? sourceFiles(url)
          : /\.(?:ts|tsx)$/.test(entry.name)
            ? [url]
            : [];
      }),
    )
  ).flat();
}

test("visible JSX text and accessibility labels belong in translation catalogs", async () => {
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
  const failures = [];
  for (const url of await sourceFiles(root)) {
    const input = await readFile(url, "utf8");
    const parsed = ts.createSourceFile(
      url.pathname,
      input,
      ts.ScriptTarget.Latest,
      true,
      url.pathname.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    );
    function visit(node) {
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
          `${url.pathname}:${parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line + 1}: ${visible.trim()}`,
        );
      ts.forEachChild(node, visit);
    }
    visit(parsed);
  }
  assert.deepEqual(failures, []);
  assert.ok(Object.keys(en).length > 10);
});
