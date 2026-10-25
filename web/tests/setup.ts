import { afterEach, beforeEach } from "vitest";
import { cleanup } from "@testing-library/react";
import i18n from "../src/i18n";

beforeEach(async () => {
  await i18n.init({
    ...i18n.options,
    lng: "en",
    supportedLngs: ["en", "test"],
  });
});

afterEach(async () => {
  cleanup();
  i18n.removeResourceBundle("test", "translation");
  await i18n.changeLanguage("en");
});
