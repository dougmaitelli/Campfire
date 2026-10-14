export type SparkKind = "local" | "remote";

export interface Spark {
  id: string;
  name: string;
  kind: SparkKind;
  created_at: string;
}

export interface CreateSparkInput {
  name: string;
  kind: SparkKind;
}

function isSpark(value: unknown): value is Spark {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof value.id === "string" &&
    "name" in value &&
    typeof value.name === "string" &&
    "kind" in value &&
    (value.kind === "local" || value.kind === "remote") &&
    "created_at" in value &&
    typeof value.created_at === "string" &&
    Number.isFinite(Date.parse(value.created_at))
  );
}

async function readResponse(response: Response): Promise<unknown> {
  const body: unknown = await response.json();
  if (!response.ok) {
    const message =
      typeof body === "object" &&
      body !== null &&
      "error" in body &&
      typeof body.error === "string"
        ? body.error
        : "Unable to complete the request.";
    throw new Error(message);
  }
  return body;
}

export async function listSparks(signal: AbortSignal): Promise<Spark[]> {
  const response = await fetch("/api/v1/sparks", { signal });
  const body = await readResponse(response);
  if (!Array.isArray(body) || !body.every(isSpark)) {
    throw new Error("Unexpected response while loading Sparks.");
  }
  return body;
}

export async function createSpark(input: CreateSparkInput): Promise<Spark> {
  const response = await fetch("/api/v1/sparks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const body = await readResponse(response);
  if (!isSpark(body)) {
    throw new Error("Unexpected response while adding the Spark.");
  }
  return body;
}
