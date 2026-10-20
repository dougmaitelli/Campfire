import i18n from "./i18n";
import "./i18n";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./style.css";

const root = document.getElementById("root");

if (!(root instanceof HTMLElement)) {
  throw new Error(i18n.t("main.unableToStartCampfireRootElementWasNotFound"));
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
