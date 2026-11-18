import { APIError } from "../../api/client";

// A lost response or server-side error after publication starts cannot prove
// whether files were saved. Both entry points must avoid a false failure/success.
export function publicationUnconfirmed(cause: unknown): boolean {
  return !(cause instanceof APIError) || cause.status >= 500;
}
