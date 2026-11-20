import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { GameAddressReading } from "../../src/components/servers/GameAddressReading";

test("connection previews explain inheritance, keep overrides literal, and report missing defaults", () => {
  const view = render(
    <GameAddressReading
      override=""
      host="games.example.test"
      port={25565}
      spark="Europe"
    />,
  );
  expect(screen.getByText("games.example.test:25565")).toBeDefined();
  expect(
    screen.getByText("Inherited from Europe and primary host port 25565."),
  ).toBeDefined();
  const address = "https://Play.Example.test:32100/Join?World=Case";
  view.rerender(
    <GameAddressReading
      override={address}
      host="2001:db8::1"
      port={25565}
      spark="Europe"
    />,
  );
  expect(screen.getByText(address)).toBeDefined();
  expect(screen.queryByText(/Inherited from/)).toBeNull();
  expect(screen.queryByRole("link")).toBeNull();
  view.rerender(
    <GameAddressReading
      override=""
      host="2001:db8::1"
      port={25565}
      spark="Europe"
    />,
  );
  expect(screen.getByText("[2001:db8::1]:25565")).toBeDefined();
  view.rerender(
    <GameAddressReading
      override=""
      host="games.example.test"
      port={0}
      spark="Europe"
    />,
  );
  expect(
    screen.getByText(/No connection address can be derived/),
  ).toBeDefined();
});
