/**
 * Notify a user of a real-time event.
 *
 * In production, this posts to the WebSocket server's internal API.
 * The WebSocket server handles routing the message to connected clients.
 */
export async function notifyUser(
  userId: string,
  event: string,
  payload: unknown
): Promise<void> {
  const wsUrl = process.env.WS_INTERNAL_URL || "http://localhost:3002";
  try {
    await fetch(`${wsUrl}/notify`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ userId, event, payload }),
    });
  } catch {
    // WebSocket server may not be running; fail silently
  }
}
