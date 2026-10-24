import type { APIRoute } from "astro";
import icon from "../../web/public/favicon.svg?raw";

export const GET: APIRoute = () =>
  new Response(icon, { headers: { "Content-Type": "image/svg+xml" } });
