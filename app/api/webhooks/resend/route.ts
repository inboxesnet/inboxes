import { NextRequest, NextResponse } from "next/server";
import { createHmac, timingSafeEqual } from "crypto";
import { prisma } from "@/lib/db";

// Map Resend event types to our EmailStatus values
const DELIVERY_STATUS_MAP: Record<string, string> = {
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

interface ResendWebhookAttachment {
  filename?: string;
  content_type?: string;
  size?: number;
}

interface ResendWebhookEvent {
  type: string;
  data: {
    email_id?: string;
    from?: string;
    to?: string | string[];
    cc?: string | string[];
    subject?: string;
    html?: string;
    text?: string;
    message_id?: string;
    in_reply_to?: string;
    references?: string | string[];
    attachments?: ResendWebhookAttachment[];
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

  // Handle inbound email
  if (type === "email.received") {
    return handleInboundEmail(data);
  }

  // Handle delivery status events
  const newStatus = DELIVERY_STATUS_MAP[type];
  if (!newStatus) {
    // Unknown or unhandled event type — acknowledge gracefully
    return NextResponse.json({ received: true });
  }

  return handleDeliveryStatus(data, newStatus);
}

/**
 * Find an existing thread for a user by checking In-Reply-To and References headers.
 * Matches against message_id on any Email (inbound or outbound) belonging to the user.
 */
async function findExistingThread(
  userId: string,
  inReplyTo: string | null,
  referencesArray: string[]
): Promise<string | null> {
  // Collect all message IDs to search for (In-Reply-To + References)
  const messageIdsToCheck: string[] = [];
  if (inReplyTo) {
    messageIdsToCheck.push(inReplyTo);
  }
  // Add references in reverse order (most recent first) for better matching
  for (let i = referencesArray.length - 1; i >= 0; i--) {
    if (!messageIdsToCheck.includes(referencesArray[i])) {
      messageIdsToCheck.push(referencesArray[i]);
    }
  }

  if (messageIdsToCheck.length === 0) {
    return null;
  }

  // Find any email belonging to this user that matches one of the message IDs
  const matchingEmail = await prisma.email.findFirst({
    where: {
      user_id: userId,
      message_id: { in: messageIdsToCheck },
    },
    select: { thread_id: true },
    orderBy: { created_at: "desc" },
  });

  return matchingEmail ? matchingEmail.thread_id : null;
}

/**
 * Strip Re:/Fwd:/Fw: prefixes from subject for clean thread subject.
 */
function cleanSubject(subject: string): string {
  return subject.replace(/^(Re|Fwd|Fw):\s*/gi, "").trim() || "(No Subject)";
}

/**
 * Deliver an inbound email to a specific user (creating/updating thread).
 * Returns true if the email was delivered, false if it was a duplicate.
 */
async function deliverToUser(
  user: { id: string; org_id: string },
  opts: {
    fromAddress: string;
    toAddresses: string[];
    ccAddresses: string[];
    subject: string;
    bodyHtml: string;
    bodyPlain: string;
    messageId: string | null;
    inReplyTo: string | null;
    referencesArray: string[];
    attachmentsMeta: { filename: string; content_type: string; size: number }[];
    allParticipants: string[];
    aliasId?: string;
    originalTo?: string;
  }
): Promise<boolean> {
  const {
    fromAddress, toAddresses, ccAddresses, subject, bodyHtml, bodyPlain,
    messageId, inReplyTo, referencesArray, attachmentsMeta, allParticipants, aliasId, originalTo,
  } = opts;

  // Check for duplicate: skip if we already have this message_id for this user
  if (messageId) {
    const existingEmail = await prisma.email.findFirst({
      where: {
        user_id: user.id,
        message_id: messageId,
        direction: "inbound",
      },
    });
    if (existingEmail) {
      return false;
    }
  }

  // Threading: check In-Reply-To and References to find existing thread
  const existingThreadId = await findExistingThread(
    user.id,
    inReplyTo,
    referencesArray
  );

  let threadId: string;

  if (existingThreadId) {
    // Add to existing thread: update last_message_at, increment message_count and unread_count
    const existingThread = await prisma.thread.update({
      where: { id: existingThreadId },
      data: {
        last_message_at: new Date(),
        message_count: { increment: 1 },
        unread_count: { increment: 1 },
      },
    });

    // Update participant_emails with any new addresses
    let currentParticipants: string[] = [];
    try {
      const raw = existingThread.participant_emails;
      currentParticipants = typeof raw === "string" ? JSON.parse(raw) : (raw as string[]);
    } catch {
      currentParticipants = [];
    }
    const mergedParticipants = Array.from(
      new Set([...currentParticipants, ...allParticipants])
    );
    await prisma.thread.update({
      where: { id: existingThreadId },
      data: {
        participant_emails: JSON.stringify(mergedParticipants),
      },
    });

    threadId = existingThreadId;
  } else {
    // No existing thread found — create a new one
    const newThread = await prisma.thread.create({
      data: {
        org_id: user.org_id,
        user_id: user.id,
        subject: cleanSubject(subject),
        participant_emails: JSON.stringify(allParticipants),
        last_message_at: new Date(),
        message_count: 1,
        unread_count: 1,
        folder: "inbox",
      },
    });
    threadId = newThread.id;
  }

  // Create Email record
  await prisma.email.create({
    data: {
      org_id: user.org_id,
      thread_id: threadId,
      user_id: user.id,
      message_id: messageId,
      in_reply_to: inReplyTo,
      references_header: referencesArray.length > 0 ? referencesArray : undefined,
      from_address: fromAddress,
      to_addresses: JSON.stringify(toAddresses),
      cc_addresses: JSON.stringify(ccAddresses),
      bcc_addresses: JSON.stringify([]),
      subject,
      body_html: bodyHtml,
      body_plain: bodyPlain,
      attachments: JSON.stringify(attachmentsMeta),
      direction: "inbound",
      status: "received",
      read: false,
      folder: "inbox",
      received_at: new Date(),
      delivered_via_alias: aliasId || null,
      original_to: originalTo || null,
    },
  });

  return true;
}

async function handleInboundEmail(
  data: ResendWebhookEvent["data"]
): Promise<NextResponse> {
  const fromAddress = data.from || "";
  const toRaw = data.to;
  const ccRaw = data.cc;
  const subject = data.subject || "(No Subject)";
  const bodyHtml = data.html || "";
  const bodyPlain = data.text || "";
  const messageId = data.message_id || null;
  const inReplyTo = data.in_reply_to || null;
  const referencesRaw = data.references;
  const attachmentsRaw = data.attachments || [];

  // Normalize to/cc to arrays
  const toAddresses = toRaw
    ? (Array.isArray(toRaw) ? toRaw : [toRaw])
    : [];
  const ccAddresses = ccRaw
    ? (Array.isArray(ccRaw) ? ccRaw : [ccRaw])
    : [];

  // Normalize references to array
  const referencesArray = referencesRaw
    ? (Array.isArray(referencesRaw) ? referencesRaw : referencesRaw.split(/\s+/).filter(Boolean))
    : [];

  // Store attachment metadata as JSON-friendly objects
  const attachmentsMeta = attachmentsRaw.map((att) => ({
    filename: att.filename || "untitled",
    content_type: att.content_type || "application/octet-stream",
    size: att.size || 0,
  }));

  if (toAddresses.length === 0) {
    // No recipients to route to — acknowledge gracefully
    return NextResponse.json({ received: true });
  }

  // All participants for this email
  const allParticipants = Array.from(
    new Set([fromAddress, ...toAddresses, ...ccAddresses])
  );

  // Track user IDs that have already received this email (prevent duplicate delivery)
  const deliveredUserIds = new Set<string>();

  const deliverOpts = {
    fromAddress, toAddresses, ccAddresses, subject, bodyHtml, bodyPlain,
    messageId, inReplyTo, referencesArray, attachmentsMeta, allParticipants,
  };

  // Phase 1: Direct user matching — route to users whose email matches a `to` address
  for (const toAddr of toAddresses) {
    // Extract email address from "Name <email>" format if needed
    const emailMatch = toAddr.match(/<([^>]+)>/);
    const normalizedTo = emailMatch ? emailMatch[1].toLowerCase() : toAddr.toLowerCase().trim();

    // Find user by email address
    const user = await prisma.user.findFirst({
      where: {
        email: normalizedTo,
        status: "active",
      },
    });

    if (!user) {
      continue;
    }

    const delivered = await deliverToUser(user, deliverOpts);
    if (delivered) {
      deliveredUserIds.add(user.id);
    }
  }

  // Phase 2: Alias matching — route to alias members if `to` matches an alias address
  // Also track which addresses were matched (user or alias) for catch-all fallback
  const matchedAddresses = new Set<string>();

  for (const toAddr of toAddresses) {
    const emailMatch = toAddr.match(/<([^>]+)>/);
    const normalizedTo = emailMatch ? emailMatch[1].toLowerCase() : toAddr.toLowerCase().trim();

    // Check if this address was already matched to a direct user in Phase 1
    const directUser = await prisma.user.findFirst({
      where: { email: normalizedTo, status: "active" },
      select: { id: true },
    });
    if (directUser) {
      matchedAddresses.add(normalizedTo);
    }

    // Find alias by address
    const alias = await prisma.alias.findFirst({
      where: {
        address: normalizedTo,
      },
      include: {
        alias_users: {
          include: {
            user: true,
          },
        },
      },
    });

    if (!alias) {
      continue;
    }

    matchedAddresses.add(normalizedTo);

    // Deliver to each alias user who hasn't already received this email
    for (const aliasUser of alias.alias_users) {
      if (deliveredUserIds.has(aliasUser.user_id)) {
        // User already received via direct addressing — skip to avoid duplicates
        continue;
      }

      if (aliasUser.user.status !== "active") {
        continue;
      }

      const delivered = await deliverToUser(
        { id: aliasUser.user_id, org_id: alias.org_id },
        { ...deliverOpts, aliasId: alias.id }
      );
      if (delivered) {
        deliveredUserIds.add(aliasUser.user_id);
      }
    }
  }

  // Phase 3: Catch-all routing — deliver to org admins if address matches no user and no alias
  for (const toAddr of toAddresses) {
    const emailMatch = toAddr.match(/<([^>]+)>/);
    const normalizedTo = emailMatch ? emailMatch[1].toLowerCase() : toAddr.toLowerCase().trim();

    // Skip if this address was already matched to a user or alias
    if (matchedAddresses.has(normalizedTo)) {
      continue;
    }

    // Extract domain from the email address
    const atIndex = normalizedTo.indexOf("@");
    if (atIndex === -1) continue;
    const emailDomain = normalizedTo.slice(atIndex + 1);

    // Find the domain record and its org
    const domain = await prisma.domain.findFirst({
      where: { domain: emailDomain },
      include: { org: true },
    });

    if (!domain) {
      // Not our domain — skip
      continue;
    }

    if (!domain.org.catch_all_enabled) {
      // Catch-all disabled — email will bounce (Resend handles bounce)
      continue;
    }

    // Deliver to all active admins in this org
    const admins = await prisma.user.findMany({
      where: {
        org_id: domain.org_id,
        role: "admin",
        status: "active",
      },
    });

    for (const admin of admins) {
      if (deliveredUserIds.has(admin.id)) {
        continue;
      }

      const delivered = await deliverToUser(
        { id: admin.id, org_id: domain.org_id },
        { ...deliverOpts, originalTo: normalizedTo }
      );
      if (delivered) {
        deliveredUserIds.add(admin.id);
      }
    }
  }

  return NextResponse.json({ received: true });
}

async function handleDeliveryStatus(
  data: ResendWebhookEvent["data"],
  newStatus: string
): Promise<NextResponse> {
  const emailId = data.email_id;
  if (!emailId) {
    // Cannot match without email_id — ignore gracefully
    return NextResponse.json({ received: true });
  }

  // Try to find the email by message_id or direct id match
  const email = await prisma.email.findFirst({
    where: {
      direction: "outbound",
      OR: [
        { message_id: { contains: emailId } },
        { id: emailId },
      ],
    },
    orderBy: { created_at: "desc" },
  });

  if (!email) {
    // Email not found — ignore gracefully
    return NextResponse.json({ received: true });
  }

  // Check if status already matches (idempotency)
  if (email.status === newStatus) {
    return NextResponse.json({ received: true });
  }

  // Don't downgrade status
  const STATUS_PRIORITY: Record<string, number> = {
    sent: 1,
    delivered: 2,
    bounced: 3,
    failed: 4,
  };

  const currentPriority = STATUS_PRIORITY[email.status] || 0;
  const newPriority = STATUS_PRIORITY[newStatus] || 0;

  if (newPriority <= currentPriority) {
    return NextResponse.json({ received: true });
  }

  // Update the email status
  await prisma.email.update({
    where: { id: email.id },
    data: { status: newStatus as "sent" | "delivered" | "bounced" | "failed" },
  });

  return NextResponse.json({ received: true });
}
