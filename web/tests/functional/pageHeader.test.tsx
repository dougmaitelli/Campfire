import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { PageHeader } from "../../src/components/layout/PageHeader";

test("compact header actions retain their full accessible names and original handler", () => {
  const create = vi.fn();
  render(
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
  fireEvent.click(button);
  expect(create).toHaveBeenCalledOnce();
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

test("secondary actions retain accessible labels and callbacks", () => {
  const importEmber = vi.fn();
  render(
    <PageHeader
      title="Embers"
      secondaryActions={[
        { label: "Import Ember", icon: "upload", onSelect: importEmber },
      ]}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Import Ember" }));
  expect(importEmber).toHaveBeenCalledOnce();
  expect(screen.getByRole("button", { name: "Actions" })).toBeDefined();
});

test("headers without actions expose their content without action buttons", () => {
  render(
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
  expect(screen.queryByRole("button")).toBeNull();
});
