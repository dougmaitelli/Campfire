import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { PageHeader } from "../../src/components/layout/PageHeader";

test("compact header actions retain their full accessible names and original handler", () => {
  const create = vi.fn();
  const { container } = render(
    <PageHeader
      title="Schedules"
      description="Run server tasks automatically."
      primaryAction={{
        label: "Create schedule",
        compactLabel: "Create",
        onClick: create,
      }}
    />,
  );
  const button = screen.getByRole("button", { name: "Create schedule" });
  expect(button.title).toBe("Create schedule");
  expect(button.querySelector(".page-header-action-label")?.textContent).toBe(
    "Create schedule",
  );
  expect(
    button.querySelector(".page-header-action-compact-label")?.textContent,
  ).toBe("Create");
  expect(button.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
  fireEvent.click(button);
  expect(create).toHaveBeenCalledOnce();
  expect(container.querySelector(".page-header-content")).not.toBeNull();
  expect(screen.getByText("Run server tasks automatically.")).toBeDefined();
});

test("disabled primary actions remain unavailable", () => {
  const create = vi.fn();
  render(
    <PageHeader
      title="Users"
      primaryAction={{
        label: "Create user",
        compactLabel: "Create",
        disabled: true,
        onClick: create,
      }}
    />,
  );
  const button = screen.getByRole<HTMLButtonElement>("button", {
    name: "Create user",
  });
  expect(button.disabled).toBe(true);
  fireEvent.click(button);
  expect(create).not.toHaveBeenCalled();
});

test("secondary actions support a desktop button and the shared mobile menu", () => {
  const importEmber = vi.fn();
  const { container } = render(
    <PageHeader
      title="Embers"
      secondaryActions={[
        { label: "Import Ember", icon: "upload", onSelect: importEmber },
      ]}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Import Ember" }));
  expect(importEmber).toHaveBeenCalledOnce();
  expect(
    container
      .querySelector(".page-header-secondary-menu button")
      ?.getAttribute("aria-label"),
  ).toBe("Actions");
});

test("headers without actions keep their title, breadcrumb, badge and description without an empty action group", () => {
  const { container } = render(
    <PageHeader
      title="Survival"
      leading={<nav aria-label="Breadcrumb">Servers</nav>}
      badge={<span>Running</span>}
      description="Europe · Valheim"
    />,
  );
  expect(
    screen.getByRole("heading", { name: "Survival", level: 1 }),
  ).toBeDefined();
  expect(screen.getByRole("navigation", { name: "Breadcrumb" })).toBeDefined();
  expect(screen.getByText("Running")).toBeDefined();
  expect(screen.getByText("Europe · Valheim")).toBeDefined();
  expect(container.querySelector(".page-actions")).toBeNull();
});
