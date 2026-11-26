import { expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ServerConsole } from "../../src/components/servers/ServerConsole";
import type { Server } from "../../src/api/servers";
import { Permission } from "../../src/contracts/permissions";

vi.mock("../../src/hooks/useNavigationGuard", () => ({
  useNavigationGuard: vi.fn(),
}));
vi.mock("../../src/components/servers/ServerTerminal", () => ({
  ServerTerminal: ({ canWrite }: { canWrite: boolean }) => (
    <div>
      Console output
      {canWrite && <input aria-label="Command" />}
    </div>
  ),
}));

test.each([
  [undefined, true, true],
  [true, true, true],
  [false, true, false],
  [undefined, false, false],
  [true, false, false],
  [false, false, false],
])(
  "console capability %s and write permission %s yield command input %s",
  async (enabled, write, visible) => {
    const server = {
      id: "game",
      permissions: [
        Permission.ServerView,
        Permission.ServerConsoleRead,
        ...(write ? [Permission.ServerConsoleWrite] : []),
      ],
      ...(enabled === undefined ? {} : { console_commands: enabled }),
    } as Server;
    render(<ServerConsole server={server} />);
    await screen.findByText("Console output");
    expect(screen.queryByRole("textbox", { name: "Command" }) !== null).toBe(
      visible,
    );
  },
);
