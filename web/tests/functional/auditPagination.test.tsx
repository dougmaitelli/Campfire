import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import { AuditPage } from "../../src/pages/AuditPage";
import { listAudit } from "../../src/api/activity";
import type * as ActivityAPI from "../../src/api/activity";
import { replaceCurrentRoute } from "../../src/routing/useCurrentPage";
import { pageFixture } from "../pageFixture";

vi.mock("../../src/hooks/useAuthentication", () => ({
  useAuthentication: () => ({ session: null }),
}));
vi.mock("../../src/api/activity", async (original) => ({
  ...(await original<typeof ActivityAPI>()),
  listAudit: vi.fn(),
}));

beforeEach(() => {
  vi.resetAllMocks();
  replaceCurrentRoute("audit");
  vi.mocked(listAudit).mockImplementation(async (_signal, page) =>
    pageFixture([], page),
  );
});

test("URL navigation resets submitted and unsaved Audit drafts to the active filters", async () => {
  const user = userEvent.setup();
  render(<AuditPage />);
  const [action, target] = screen.getAllByRole<HTMLInputElement>("textbox");
  await user.type(action!, "server.restart");
  await user.type(target!, "server:game");
  await user.click(screen.getByRole("button", { name: "Filter" }));
  await waitFor(() =>
    expect(listAudit).toHaveBeenLastCalledWith(
      expect.anything(),
      1,
      "server.restart",
      "server:game",
    ),
  );
  await user.clear(action!);
  await user.type(action!, "unsaved");
  await act(async () => replaceCurrentRoute("audit"));
  await waitFor(() =>
    expect(listAudit).toHaveBeenLastCalledWith(expect.anything(), 1, "", ""),
  );
  expect(action!.value).toBe("");
  expect(target!.value).toBe("");
  await act(async () =>
    replaceCurrentRoute(
      "audit?action=server.restart&target=server%3Agame&page=2",
    ),
  );
  await waitFor(() =>
    expect(listAudit).toHaveBeenLastCalledWith(
      expect.anything(),
      2,
      "server.restart",
      "server:game",
    ),
  );
  expect(action!.value).toBe("server.restart");
  expect(target!.value).toBe("server:game");
});
