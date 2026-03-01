# WebSocket Events

Real-time event system for live updates across all connected clients.

## Connection

**Endpoint:** `GET /api/ws`

**Auth:** JWT from `httpOnly` cookie named `token`. No query-string tokens.

**Origin validation:** The server compares the `Origin` header against the configured `APP_URL`. Non-browser clients (no `Origin` header) are allowed.

**Heartbeat:** The server sends WebSocket `ping` frames every 30 seconds. If no `pong` is received within 60 seconds, the connection is closed.

**Token validation:** Every 5 minutes the server checks whether the JWT has expired or been revoked (via token blacklist). If so, the connection is closed with a WebSocket close frame.

**Alias refresh:** Every 5 minutes the server reloads the client's alias assignments from the database, ensuring event filtering stays current if aliases change.

## Message Format

All messages are JSON:

```json
{
  "event": "thread.starred",
  "thread_id": "uuid",
  "domain_id": "uuid",
  "payload": { ... }
}
```

The `payload` field varies by event type. `thread_id` and `domain_id` may be empty for non-thread events.

## Event Types

### Email Events

| Event | Description | Payload |
|-------|-------------|---------|
| `email.received` | New inbound email arrived | — |
| `email.sent` | Outbound email sent | — |
| `email.status_updated` | Delivery status changed | `{ "status": "delivered\|bounced\|failed", "subject": "..." }` |

### Thread Events

| Event | Description | Payload |
|-------|-------------|---------|
| `thread.starred` | Thread starred | `{ "thread": { ...thread } }` |
| `thread.unstarred` | Thread unstarred | `{ "thread": { ...thread } }` |
| `thread.muted` | Thread muted | `{ "thread": { ...thread } }` |
| `thread.unmuted` | Thread unmuted | `{ "thread": { ...thread } }` |
| `thread.read` | Thread marked read | — |
| `thread.unread` | Thread marked unread | — |
| `thread.archived` | Thread archived | `{ "thread": { ...thread } }` |
| `thread.trashed` | Thread trashed | `{ "thread": { ...thread } }` |
| `thread.spammed` | Thread moved to spam | `{ "thread": { ...thread } }` |
| `thread.moved` | Thread moved to label | `{ "thread": { ...thread } }` |
| `thread.deleted` | Thread permanently deleted | — |
| `thread.bulk_action` | Bulk action on threads | — |

### System Events

| Event | Description | Payload |
|-------|-------------|---------|
| `sync.completed` | Email sync job finished | — |
| `plan.changed` | Subscription plan changed | `{ "plan": "pro\|cancelled" }` |

## Event Filtering

Events are scoped to the user's organization. Two filtering modes:

### Broadcast Events (all org members)

These events are always sent to every connected client in the org:

- `sync.completed`
- `plan.changed`
- `thread.bulk_action`

### Alias-Filtered Events (thread-specific)

For thread-specific events (`thread.starred`, `email.received`, etc.), the server checks which alias addresses are associated with the thread via `thread_labels` (`alias:user@domain.com` labels).

- **Admins** always receive all events regardless of alias assignment
- **Members** only receive events for threads that involve aliases assigned to them
- **Threads without alias labels** fall back to broadcast (all org members)

## Reconnection & Catch-Up

### Event Persistence

All events are durably stored in PostgreSQL (the `events` table) before being published to Redis for WebSocket delivery. Redis pub/sub is best-effort — if Redis is temporarily down, events are not lost.

### Reconnection with Catch-Up

1. The frontend tracks the last received event `id`
2. On WebSocket reconnect, it calls `GET /api/events?since={lastEventId}` to fetch missed events
3. The catch-up response returns events in order, allowing the frontend to replay them
4. The frontend applies the same cache update logic to catch-up events as live events

### Exponential Backoff

The frontend reconnects with exponential backoff:
- Base delay: 1 second
- Maximum delay: 30 seconds
- Multiplied by 2 on each failed attempt
- Reset to base delay on successful connection

## Cross-Tab Sync

The frontend uses the `BroadcastChannel` API (channel name: `inboxes-cache-sync`) to synchronize cache invalidations across browser tabs.

When a mutation occurs in one tab (e.g., starring a thread), the tab posts the affected React Query cache keys to the `BroadcastChannel`. Other tabs receive the message and invalidate those cache keys, triggering a refetch.

This works independently from WebSocket events and handles the case where a user takes an action in one tab and expects to see it reflected in another tab immediately.

## Event Retention

Events are pruned by the Event Pruner worker. The retention period is configured via `EVENT_RETENTION_DAYS` (default: 90 days). Pruning runs every 6 hours in batches of 5,000 rows to avoid long-running transactions.
