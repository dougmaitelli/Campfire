import eslint from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";
import { dirname } from "node:path";

const basePath = dirname(import.meta.dirname);

export default tseslint.config(
  {
    ignores: ["**/dist/**", "**/node_modules/**"],
  },
  {
    basePath,
    files: ["web/src/**/*.{ts,tsx}", "web/vite.config.ts"],
    extends: [
      eslint.configs.recommended,
      ...tseslint.configs.recommendedTypeChecked,
      reactHooks.configs.flat.recommended,
    ],
    languageOptions: {
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      "@typescript-eslint/consistent-type-imports": "error",
      "@typescript-eslint/no-floating-promises": "error",
      "@typescript-eslint/no-misused-promises": "error",
      "@typescript-eslint/no-unnecessary-condition": "error",
      "@typescript-eslint/prefer-nullish-coalescing": "error",
    },
  },
  {
    basePath,
    files: ["scripts/**/*.mjs", "eslint.config.mjs", "web/eslint.config.mjs"],
    extends: [eslint.configs.recommended],
    languageOptions: {
      globals: globals.node,
    },
  },
);
