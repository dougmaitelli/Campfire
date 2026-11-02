import { expect, test } from "vitest";
import { formatCronSchedule } from "../../src/utils/formatCronSchedule";

test.each([
  ["0 5 * * *", "Daily Backup at 5am"],
  ["30 17 * * *", "Daily Backup at 5:30pm"],
  ["0 0 * * *", "Daily Backup at 12am"],
  ["0 12 * * *", "Daily Backup at 12pm"],
  ["05 05 * * *", "Daily Backup at 5:05am"],
  [" 0   5 * * * ", "Daily Backup at 5am"],
  ["0 * * * *", "Hourly Backup"],
  ["*/15 * * * *", "Backup every 15 minutes"],
  ["0 3 * * 1-5", "Backup at 3am, Monday through Friday"],
  ["0 3 * * SUN", "Backup at 3am, only on Sunday"],
  ["0 3 1 * *", "Backup at 3am, on day 1 of the month"],
  ["0 0 5 * *", "Backup at 12am, on day 5 of the month"],
  ["0 5,17 * * *", "Backup at 5am and 5pm"],
  ["0 5 * JAN,MAR *", "Backup at 5am, only in January and March"],
])("describes %s without changing its meaning", (expression, expected) => {
  expect(formatCronSchedule(expression, "Backup")).toBe(expected);
});

test.each(["", "0 5", "bad input", "0 0 5 * * *", "99 25 * * *"])(
  "incomplete or unsupported expressions stay safe while editing: %s",
  (expression) => {
    expect(formatCronSchedule(expression, "Backup")).toBe("Recurring Backup");
  },
);
