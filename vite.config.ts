import { defineConfig } from "vite-plus";

export default defineConfig({
  staged: {
    "*.{js,ts,json}": "vp check --fix",
  },
  fmt: {
    ignorePatterns: [
      ".agents/**",
      ".claude/**",
      ".github/**",
      ".vscode/**",
      "cmd/**/template/**",
      "conf/**",
      "devenv/**",
      "docker-compose.yml",
      "docs/**",
      "internal/**/prompts/**",
      "internal/**/templates/**",
      "spec/**",
      "**/*.md",
      "**/*.toml",
      "**/*.yaml",
      "**/*.yml",
    ],
  },
  lint: {
    options: { typeAware: true, typeCheck: true },
    ignorePatterns: ["node_modules/**", "dist/**", "docs/**", "spec/**"],
  },
});
