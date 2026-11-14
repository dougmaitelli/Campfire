import type { APIRoute } from "astro";
import icon from "../../web/public/favicon.png?inline";

export const GET: APIRoute = () =>
  new Response(Buffer.from(icon.slice(icon.indexOf(",") + 1), "base64"), {
    headers: { "Content-Type": "image/png" },
  });
