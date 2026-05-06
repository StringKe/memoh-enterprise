import { Elysia } from "elysia";
import { devices } from "playwright";

export function listDevices() {
  return devices;
}

export const devicesModule = new Elysia({ prefix: "/devices" }).get("/", () => {
  return listDevices();
});
