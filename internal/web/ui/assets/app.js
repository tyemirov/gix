// @ts-check

import {
  initializeApp,
  reportBootstrapFailure,
} from "./main.js";

window.addEventListener("error", (event) => {
  reportBootstrapFailure(event.error?.stack || event.message || String(event.error || ""));
});

window.addEventListener("unhandledrejection", (event) => {
  reportBootstrapFailure(event.reason?.stack || event.reason?.message || String(event.reason || ""));
});

initializeApp().catch((error) => {
  reportBootstrapFailure(String(error));
});
