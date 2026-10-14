import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./style.css";

const root = document.getElementById("root");

if (!(root instanceof HTMLElement)) {
  throw new Error("Unable to start Campfire: root element was not found");
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
