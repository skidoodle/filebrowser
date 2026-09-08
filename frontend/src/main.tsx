import { LinkProvider } from "@cloudflare/kumo";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { AppLink } from "./components/AppLink";
import { queryClient } from "./lib/queryClient";
import { QueryClientProvider } from "@tanstack/react-query";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <LinkProvider component={AppLink}>
        <App />
      </LinkProvider>
    </QueryClientProvider>
  </StrictMode>,
);
