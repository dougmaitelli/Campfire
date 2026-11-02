import { expect, test } from "vitest";
import { ScheduleAction } from "../../src/contracts/domains";
import { ResourceLimits } from "../../src/config/policies";
import { formatScheduleName } from "../../src/utils/formatScheduleName";

const localTimeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

test("daily schedule names read as a sentence with the server and action", () => {
  expect(
    formatScheduleName({
      serverName: "Minecraft",
      action: ScheduleAction.Backup,
      timing: { once: false, expression: "0 5 * * *", timeZone: localTimeZone },
    }),
  ).toBe("Minecraft - Daily Backup at 5am");
});

test("custom expressions use readable timing and retain a different time zone", () => {
  expect(
    formatScheduleName({
      serverName: "Creative",
      action: ScheduleAction.Command,
      timing: {
        once: false,
        expression: " */5  2 * * 1 ",
        timeZone: "Europe/Paris",
      },
    }),
  ).toBe(
    "Creative - Console command every 5 minutes, between 2am and 2:59am, only on Monday (Europe/Paris)",
  );
});

test("one-off schedule names use the selected local date and time", () => {
  const name = formatScheduleName({
    serverName: "Survival",
    action: ScheduleAction.Backup,
    timing: { once: true, runAt: "2026-11-02T14:30" },
  });
  expect(name).toContain("Survival - Backup - Nov 2, 2026");
  expect(name).toContain("2:30 PM");
  expect(name).toContain(
    `(${Intl.DateTimeFormat().resolvedOptions().timeZone})`,
  );
});

test("incomplete date inputs do not throw while a schedule is being edited", () => {
  expect(
    formatScheduleName({
      serverName: "Survival",
      action: ScheduleAction.Stop,
      timing: { once: true, runAt: "" },
    }),
  ).toBe("Survival - Stop - Run once");
});

test("long schedule names keep the action and timing within the name limit", () => {
  const name = formatScheduleName({
    serverName: "🌲".repeat(ResourceLimits.NameLength),
    action: ScheduleAction.Start,
    timing: { once: false, expression: "0 3 * * *", timeZone: localTimeZone },
  });
  expect(Array.from(name)).toHaveLength(ResourceLimits.NameLength);
  expect(name).toContain("… - Daily Start at 3am");
  expect(name).not.toContain("\uFFFD");
});

test("missing selections do not generate misleading schedule names", () => {
  const timing = { once: true, runAt: "2026-11-02T14:30" } as const;
  expect(
    formatScheduleName({
      serverName: "",
      action: ScheduleAction.Backup,
      timing,
    }),
  ).toBe("");
  expect(
    formatScheduleName({ serverName: "Survival", action: "", timing }),
  ).toBe("");
});
