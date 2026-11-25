import i18n from "../i18n";
import { isUser } from "./access";
import { apiFetch, apiJSON, readJSON } from "./client";
import { isRecord } from "../utils/validation";

export async function readProfile(signal: AbortSignal) {
  const body = await readJSON(
    await apiFetch("/api/v1/auth/profile", { signal }),
  );
  if (
    !isRecord(body) ||
    !isUser(body.user) ||
    typeof body.local_password !== "boolean"
  )
    throw new Error(i18n.t("access.unexpectedUserResponse"));
  return { user: body.user, localPassword: body.local_password };
}
export async function saveProfile(
  name: string | undefined,
  email: string | undefined,
  password: string,
  currentPassword: string,
  code: string,
) {
  const body = await readJSON(
    await apiJSON(
      "/api/v1/auth/profile",
      { name, email, password, current_password: currentPassword, code },
      { method: "PUT" },
    ),
  );
  if (!isUser(body)) throw new Error(i18n.t("access.unexpectedUserResponse"));
  return body;
}
