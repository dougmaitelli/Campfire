import { Fragment } from "react";
import { expect, test, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Trans } from "react-i18next";
import i18n from "../../src/i18n";
import { ServerLifecycleControls } from "../../src/components/servers/ServerLifecycleControls";
import { runtimeStatus } from "../fixtures";

test("language changes update UI labels without changing protocol actions", async () => {
  const action = vi.fn();
  render(
    <ServerLifecycleControls
      status={runtimeStatus({ state: "stopped" })}
      disabled={false}
      canReinstall
      onAction={action}
    />,
  );
  expect(screen.getByRole("button", { name: "Start" })).toBeDefined();
  i18n.options.supportedLngs = ["en", "test"];
  i18n.addResourceBundle(
    "test",
    "translation",
    { common: { start: "Démarrer" } },
    true,
    true,
  );
  await act(() => i18n.changeLanguage("test"));
  expect(screen.getByRole("button", { name: "Démarrer" })).toBeDefined();
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Démarrer" }));
  expect(action).toHaveBeenCalledWith("start");
  expect(document.documentElement.lang).toBe("test");
});

test("translated interpolation renders untrusted content as text", () => {
  const name = '<img src=x onerror="alert(1)">';
  const view = render(
    <span>{i18n.t("users.displayLabel", { name, username: "alex" })}</span>,
  );
  expect(view.container.querySelector("img")).toBeNull();
  expect(view.container.textContent).toBe(`${name} (alex)`);
});

test("rich-text translations preserve links and escaped text in translated order", () => {
  i18n.addResourceBundle(
    "test",
    "translation",
    {
      overviewEmptyState: { description: "<reference/> — details for <name/>" },
    },
    true,
    true,
  );
  render(
    <Trans
      i18n={i18n}
      t={i18n.getFixedT("test")}
      i18nKey="overviewEmptyState.description"
      components={{
        reference: <a href="#sparks/node">Node</a>,
        name: <Fragment>{"<name>"}</Fragment>,
      }}
    />,
  );
  expect(screen.getByRole("link", { name: "Node" }).getAttribute("href")).toBe(
    "#sparks/node",
  );
  expect(document.body.textContent).toBe("Node — details for <name>");
  expect(document.querySelector("name")).toBeNull();
});
