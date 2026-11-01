import i18n from "../i18n";

export const cronPresets = [
  {
    get label() {
      return i18n.t("cronField.every15Minutes");
    },
    value: "*/15 * * * *",
    get description() {
      return i18n.t("cronField.every15Minutes");
    },
  },
  {
    get label() {
      return i18n.t("cronField.hourly");
    },
    value: "0 * * * *",
    get description() {
      return i18n.t("cronField.everyHourOnTheHour");
    },
  },
  {
    get label() {
      return i18n.t("cronField.daily");
    },
    value: "0 3 * * *",
    get description() {
      return i18n.t("cronField.everyDayAt0300");
    },
  },
  {
    get label() {
      return i18n.t("cronField.weekdays");
    },
    value: "0 3 * * 1-5",
    get description() {
      return i18n.t("cronField.mondayThroughFridayAt0300");
    },
  },
  {
    get label() {
      return i18n.t("cronField.weekly");
    },
    value: "0 3 * * 0",
    get description() {
      return i18n.t("cronField.everySundayAt0300");
    },
  },
  {
    get label() {
      return i18n.t("cronField.monthly");
    },
    value: "0 3 1 * *",
    get description() {
      return i18n.t("cronField.onTheFirstDayOfEachMonthAt0300");
    },
  },
];
