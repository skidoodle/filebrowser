import { writeFileSync } from "node:fs";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

/**
 * Recreates frontend/dist/.gitkeep after builds. The Go embed directive
 * (`all:frontend/dist`) requires the directory to exist on fresh checkouts
 * even before the first frontend build; .gitignore keeps only this file.
 */
function keepGitkeep(): Plugin {
  return {
    name: "keep-gitkeep",
    closeBundle() {
      writeFileSync(new URL("./dist/.gitkeep", import.meta.url), "");
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), keepGitkeep()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
    rolldownOptions: {
      output: {
        chunkFileNames: "assets/[hash].js",
        codeSplitting: {
          groups: [
            {
              name: "vendor",
              test: /node_modules/,
              entriesAware: true,
            },
          ],
        },
      },
    },
  },
});
