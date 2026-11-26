import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  NotificationsProvider,
  useNotifications,
} from "../../src/hooks/useNotifications";
import { NOTIFICATIONS_REFRESH_INTERVAL_MS } from "../../src/api/notifications";

function Counter() {
  const { unread, refresh } = useNotifications();
  return (
    <>
      <output aria-label="Unread count">{unread ?? "unknown"}</output>
      <button onClick={refresh}>Refresh</button>
    </>
  );
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

async function mount() {
  await act(async () => {
    render(
      <NotificationsProvider>
        <Counter />
      </NotificationsProvider>,
    );
  });
}
async function poll() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(NOTIFICATIONS_REFRESH_INTERVAL_MS);
  });
}

test("notification count survives failed automatic and explicit refreshes and recovers to zero", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(Response.json({ unread: 7 }))
    .mockRejectedValueOnce(new TypeError("offline"))
    .mockResolvedValueOnce(
      Response.json({ error: "unavailable" }, { status: 503 }),
    )
    .mockResolvedValueOnce(Response.json({ unread: 0 }));
  await mount();
  expect(screen.getByLabelText("Unread count").textContent).toBe("7");
  await poll();
  expect(screen.getByLabelText("Unread count").textContent).toBe("7");
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  });
  expect(screen.getByLabelText("Unread count").textContent).toBe("7");
  await poll();
  expect(screen.getByLabelText("Unread count").textContent).toBe("0");
  expect(fetch).toHaveBeenCalledTimes(4);
});

test("initial failure leaves the count unknown until a successful poll", async () => {
  vi.spyOn(globalThis, "fetch")
    .mockRejectedValueOnce(new TypeError("offline"))
    .mockResolvedValueOnce(Response.json({ unread: 3 }));
  await mount();
  expect(screen.getByLabelText("Unread count").textContent).toBe("unknown");
  await poll();
  expect(screen.getByLabelText("Unread count").textContent).toBe("3");
});
