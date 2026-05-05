import dotenv from "dotenv";
import { defineConfig } from "vite-plus";

dotenv.config();

export default defineConfig({
  test: {
    globals: true,
    include: ["packages/**/*.test.ts", "apps/**/*.test.ts"],
    env: process.env,
    passWithNoTests: true,
    testTimeout: Infinity,
  },
});
