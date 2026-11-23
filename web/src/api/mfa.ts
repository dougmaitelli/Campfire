import i18n from "../i18n";
import { apiFetch, apiJSON, readJSON } from "./client";
import { isRecord } from "../utils/validation";

export interface MFAStatus {
  enabled: boolean;
  available: boolean;
  recovery_remaining: number;
  devices: MFADevice[];
}
export interface MFADevice {
  id: string;
  name: string;
  created_at: number;
}
export interface MFAEnrollment {
  secret: string;
  uri: string;
  qr_code: string;
  expires_at: number;
}
function unexpected(): never {
  throw new Error(i18n.t("mfa.unexpectedResponse"));
}
export async function readMFA(signal: AbortSignal): Promise<MFAStatus> {
  const body = await readJSON(await apiFetch("/api/v1/auth/mfa", { signal }));
  if (
    !isRecord(body) ||
    typeof body.enabled !== "boolean" ||
    typeof body.available !== "boolean" ||
    typeof body.recovery_remaining !== "number" ||
    !Array.isArray(body.devices) ||
    !body.devices.every(
      (device): device is MFADevice =>
        isRecord(device) &&
        typeof device.id === "string" &&
        typeof device.name === "string" &&
        typeof device.created_at === "number",
    )
  )
    return unexpected();
  return {
    enabled: body.enabled,
    available: body.available,
    recovery_remaining: body.recovery_remaining,
    devices: body.devices,
  };
}
export async function beginMFA(name: string): Promise<MFAEnrollment> {
  const body = await readJSON(
    await apiJSON("/api/v1/auth/mfa/enroll", { name }),
  );
  if (
    !isRecord(body) ||
    typeof body.secret !== "string" ||
    typeof body.uri !== "string" ||
    typeof body.qr_code !== "string" ||
    !body.qr_code.startsWith("data:image/png;base64,") ||
    typeof body.expires_at !== "number"
  )
    return unexpected();
  return {
    secret: body.secret,
    uri: body.uri,
    qr_code: body.qr_code,
    expires_at: body.expires_at,
  };
}
export async function confirmMFA(code: string): Promise<string[]> {
  const body = await readJSON(
    await apiJSON("/api/v1/auth/mfa/confirm", { code }),
  );
  if (
    !isRecord(body) ||
    !Array.isArray(body.recovery_codes) ||
    !body.recovery_codes.every(
      (value): value is string => typeof value === "string",
    )
  )
    return unexpected();
  return body.recovery_codes;
}
export async function disableMFA(
  password: string,
  code: string,
): Promise<void> {
  const response = await apiJSON(
    "/api/v1/auth/mfa",
    { password, code },
    { method: "DELETE" },
  );
  if (response.status !== 204) await readJSON(response);
}
export async function resetUserMFA(id: number): Promise<void> {
  const response = await apiJSON(`/api/v1/users/${id}/reset-2fa`, {});
  if (response.status !== 204) await readJSON(response);
}
export async function removeMFADevice(id: string, code: string): Promise<void> {
  const response = await apiJSON(
    `/api/v1/auth/mfa/devices/${encodeURIComponent(id)}`,
    { code },
    { method: "DELETE" },
  );
  if (response.status !== 204) await readJSON(response);
}
