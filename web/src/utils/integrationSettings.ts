import type { ServerIntegrationInput } from "../api/integrations";

// Keep changes that only stop existing publishing available while Discord is
// unreachable or its permissions have been revoked. The backend enforces this
// same rule before authorizing an enabled destination or new publishing work.
export function needsIntegrationPublishingCheck(
  previous: ServerIntegrationInput | null,
  next: ServerIntegrationInput,
) {
  if (!next.enabled) return false;
  if (
    !previous ||
    !previous.enabled ||
    previous.integration_id !== next.integration_id ||
    previous.channel_id !== next.channel_id
  )
    return true;
  if (
    next.card.enabled &&
    (!previous.card.enabled ||
      JSON.stringify(previous.card.fields) !== JSON.stringify(next.card.fields))
  )
    return true;
  return next.subscriptions.some(
    (subscription) =>
      !previous.subscriptions.some(
        (old) => JSON.stringify(old) === JSON.stringify(subscription),
      ),
  );
}
