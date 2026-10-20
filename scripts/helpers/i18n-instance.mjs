import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";

const require = createRequire(
  new URL("../../web/package.json", import.meta.url),
);
const i18next = require("i18next");
const { initReactI18next } = require("react-i18next");
const en = JSON.parse(
  await readFile(
    new URL("../../web/src/i18n/locales/en.json", import.meta.url),
    "utf8",
  ),
);
const i18n = i18next.createInstance();
await i18n.use(initReactI18next).init({
  lng: "en",
  fallbackLng: "en",
  resources: { en: { translation: en } },
  interpolation: { escapeValue: false },
});
export default i18n;
