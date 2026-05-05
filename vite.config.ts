import { defineConfig } from "vite-plus";

export default defineConfig({
  staged: {
    "*.{js,ts,json}": "vp check --fix",
  },
  fmt: {
    ignorePatterns: [
      "cmd/**/template/**",
      "conf/**",
      "deploy/compose/dev/**",
      "deploy/docker/dev/**",
      "deploy/config/dev/**",
      "docker-compose.yml",
      "docs/**",
      "internal/**/prompts/**",
      "internal/**/templates/**",
      "api/openapi/**",
      "**/*.md",
      "**/*.toml",
      "**/*.yaml",
      "**/*.yml",
    ],
  },
  lint: {
    options: { typeAware: true, typeCheck: true },
    ignorePatterns: [
      "node_modules/**",
      "dist/**",
      "docs/**",
      "api/openapi/**",
      "packages/sdk/src/**",
    ],
    rules: {
      "typescript/no-base-to-string": "off",
      "typescript/no-floating-promises": "off",
      "typescript/no-misused-spread": "off",
      "typescript/no-redundant-type-constituents": "off",
    },
  },
});
