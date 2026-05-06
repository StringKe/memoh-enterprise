import { Elysia } from "elysia";
import { getAvailableCores } from "../browser";

export function listCores() {
  return getAvailableCores();
}

export const coresModule = new Elysia({ prefix: "/cores" }).get("/", () => {
  return { cores: listCores() };
});
