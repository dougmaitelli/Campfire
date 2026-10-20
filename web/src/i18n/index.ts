import i18n from "i18next";
import { initReactI18next } from "react-i18next";

const modules = import.meta.glob<{ default: Record<string, unknown> }>(
  "./locales/*.json",
  { eager: true },
);
const resources: Record<string, { translation: Record<string, unknown> }> = {};
for (const [path, module] of Object.entries(modules)) {
  const language = path.match(/\/([^/]+)\.json$/)?.[1];
  if (language) resources[language] = { translation: module.default };
}

void i18n.use(initReactI18next).init({
  lng: navigator.language,
  fallbackLng: "en",
  supportedLngs: Object.keys(resources),
  resources,
  interpolation: { escapeValue: false },
});

function updateDocumentLanguage() {
  document.documentElement.lang = i18n.resolvedLanguage ?? "en";
}
i18n.on("languageChanged", updateDocumentLanguage);
updateDocumentLanguage();

export default i18n;
