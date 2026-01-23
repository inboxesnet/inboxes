import { NextRequest, NextResponse } from "next/server";
import { createHmac, timingSafeEqual } from "crypto";
import { prisma } from "@/lib/db";

// Map Resend event types to our EmailStatus values
const EVENT_STATUS_MAP: Record<string, string> = {
  "email.sent": "sent",
  "email.delivered": "delivered",
  "email.bounced": "bounced",
  "email.delivery_delayed": "sent", // Keep as sent, delivery is just delayed
};

function verifyWebhookSignature(
  payload: string,
  signature: string | null,
  secret: string
): boolean {
  if (!signature) return false;

  // Resend uses svix for webhooks: signature format is "v1,<base64-signature>"
  // The signature is computed as HMAC-SHA256 of "<msg_id>.<timestamp>.<body>"
  // But simplified: verify the raw body with the signing secret
  const parts = signature.split(" ");
  for (const part of parts) {
    const [version, sig] = part.split(",");
    if (version !== "v1") continue;

    const expectedSig = createHmac("sha256", secret)
      .update(payload)
      .digest("base64");

    try {
      const sigBuffer = Buffer.from(sig, "base64");
      const expectedBuffer = Buffer.from(expectedSig, "base64");
      if (sigBuffer.length === expectedBuffer.length && timingSafeEqual(sigBuffer, expectedBuffer)) {
        return true;
      }
    } catch {
      continue;
    }
  }
  return false;
}

function verifySvixSignature(
  payload: string,
  headers: {
    msgId: string | null;
    timestamp: string | null;
    signature: string | null;
  },
  secret: string
): boolean {
  const { msgId, timestamp, signature } = headers;
  if (!msgId || !timestamp || !signature) return false;

  // Check timestamp is within 5 minutes
  const timestampSeconds = parseInt(timestamp, 10);
  const now = Math.floor(Date.now() / 1000);
  if (Math.abs(now - timestampSeconds) > 300) return false;

  // Svix secret format: "whsec_<base64-key>"
  const secretBytes = secret.startsWith("whsec_")
    ? Buffer.from(secret.slice(6), "base64")
    : Buffer.from(secret);

  // Signature content: "<msg_id>.<timestamp>.<body>"
  const signContent = `${msgId}.${timestamp}.${payload}`;

  const expectedSig = createHmac("sha256", secretBytes)
    .update(signContent)
    .digest("base64");

  // Signature header can contain multiple "v1,<sig>" entries separated by space
  const signatures = signature.split(" ");
  for (const sig of signatures) {
    const [version, sigValue] = sig.split(",");
    if (version !== "v1" || !sigValue) continue;

    try {
      const sigBuffer = Buffer.from(sigValue, "base64");
      const expectedBuffer = Buffer.from(expectedSig, "base64");
      if (
        sigBuffer.length === expectedBuffer.length &&
        timingSafeEqual(sigBuffer, expectedBuffer)
      ) {
        return true;
      }
    } catch {
      continue;
    }
  }
  return false;
}

interface ResendWebhookEvent {
  type: string;
  data: {
    email_id?: string;
    from?: string;
    to?: string[];
    subject?: string;
    created_at?: string;
  };
}

export async function POST(request: NextRequest) {
  const webhookSecret = process.env.RESEND_WEBHOOK_SECRET;
  if (!webhookSecret) {
    return NextResponse.json(
      { error: "Webhook secret not configured" },
      { status: 500 }
    );
  }

  const rawBody = await request.text();

  // Verify webhook signature using Svix headers (Resend uses Svix)
  const svixId = request.headers.get("svix-id");
  const svixTimestamp = request.headers.get("svix-timestamp");
  const svixSignature = request.headers.get("svix-signature");

  const isValid = verifySvixSignature(rawBody, {
    msgId: svixId,
    timestamp: svixTimestamp,
    signature: svixSignature,
  }, webhookSecret);

  if (!isValid) {
    return NextResponse.json(
      { error: "Invalid webhook signature" },
      { status: 401 }
    );
  }

  let event: ResendWebhookEvent;
  try {
    event = JSON.parse(rawBody) as ResendWebhookEvent;
  } catch {
    return NextResponse.json(
      { error: "Invalid JSON payload" },
      { status: 400 }
    );
  }

  const { type, data } = event;

  // Only handle delivery status events
  const newStatus = EVENT_STATUS_MAP[type];
  if (!newStatus) {
    // Unknown or unhandled event type — acknowledge gracefully
    return NextResponse.json({ received: true });
  }

  // Find the email by Resend's email_id stored in message_id or by matching
  // Resend includes email_id in webhook data
  const emailId = data.email_id;
  if (!emailId) {
    // Cannot match without email_id — ignore gracefully
    return NextResponse.json({ received: true });
  }

  // Find email by message_id containing the Resend email ID
  // Our message_id format: <uuid@domain> but Resend returns their own ID
  // We need to match via the Resend response ID. Since we don't store it separately,
  // we'll find outbound emails and match by Resend's references.
  // Alternative: update the email with the closest match by email_id field
  // For now, look up by a broad match - emails where message_id contains the id
  // Actually, Resend webhook data includes the email_id which is their internal ID.
  // We should find emails that were sent recently and match.
  // The best approach: store resend_id on Email model, or match by message_id.
  // Since we generate our own message_id, we can look up by checking if the
  // Resend API response matched. For MVP, look up by the email_id from Resend.

  // Try to find the email - Resend's email_id might be stored or we match by message_id
  const email = await prisma.email.findFirst({
    where: {
      direction: "outbound",
      // Try matching message_id that contains the resend email_id
      OR: [
        { message_id: { contains: emailId } },
        { id: emailId },
      ],
    },
    orderBy: { created_at: "desc" },
  });

  if (!email) {
    // Email not found — could be a duplicate or already processed. Ignore gracefully.
    return NextResponse.json({ received: true });
  }

  // Check if status already matches (idempotency - ignore duplicate events)
  if (email.status === newStatus) {
    return NextResponse.json({ received: true });
  }

  // Don't downgrade status (e.g., don't go from delivered back to sent)
  const STATUS_PRIORITY: Record<string, number> = {
    sent: 1,
    delivered: 2,
    bounced: 3,
    failed: 4,
  };

  const currentPriority = STATUS_PRIORITY[email.status] || 0;
  const newPriority = STATUS_PRIORITY[newStatus] || 0;

  if (newPriority <= currentPriority) {
    // Don't downgrade status
    return NextResponse.json({ received: true });
  }

  // Update the email status
  await prisma.email.update({
    where: { id: email.id },
    data: { status: newStatus as "sent" | "delivered" | "bounced" | "failed" },
  });

  return NextResponse.json({ received: true });
}
