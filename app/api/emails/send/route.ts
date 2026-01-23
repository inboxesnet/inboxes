import { NextRequest, NextResponse } from "next/server";
import { Prisma } from "@prisma/client";
import { prisma } from "@/lib/db";
import { getCurrentUser } from "@/lib/session";
import { randomUUID } from "crypto";

export async function POST(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Check org has an active domain
  const domain = await prisma.domain.findFirst({
    where: { org_id: user.org_id, status: "active" },
  });

  if (!domain) {
    return NextResponse.json(
      { error: "Organization domain is not active. Please verify your domain first." },
      { status: 400 }
    );
  }

  let body: {
    to?: string | string[];
    cc?: string | string[];
    bcc?: string | string[];
    subject?: string;
    body_html?: string;
    body_plain?: string;
    in_reply_to?: string;
    references?: string[];
  };

  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { to, cc, bcc, subject, body_html, body_plain, in_reply_to, references } = body;

  // Validate required fields
  if (!to) {
    return NextResponse.json({ error: "to is required" }, { status: 400 });
  }
  if (!subject || typeof subject !== "string") {
    return NextResponse.json({ error: "subject is required" }, { status: 400 });
  }
  if (!body_html && !body_plain) {
    return NextResponse.json({ error: "body_html or body_plain is required" }, { status: 400 });
  }

  // Normalize recipients to arrays
  const toAddresses = Array.isArray(to) ? to : [to];
  const ccAddresses = cc ? (Array.isArray(cc) ? cc : [cc]) : [];
  const bccAddresses = bcc ? (Array.isArray(bcc) ? bcc : [bcc]) : [];

  // Generate a Message-ID
  const messageId = `<${randomUUID()}@${domain.domain}>`;

  // Send via Resend API
  const resendApiKey = process.env.RESEND_API_KEY;
  if (!resendApiKey) {
    return NextResponse.json(
      { error: "Email service not configured" },
      { status: 500 }
    );
  }

  const resendPayload: Record<string, unknown> = {
    from: user.email,
    to: toAddresses,
    subject,
    headers: {
      "Message-ID": messageId,
    },
  };

  if (ccAddresses.length > 0) resendPayload.cc = ccAddresses;
  if (bccAddresses.length > 0) resendPayload.bcc = bccAddresses;
  if (body_html) resendPayload.html = body_html;
  if (body_plain) resendPayload.text = body_plain;
  if (in_reply_to) {
    (resendPayload.headers as Record<string, string>)["In-Reply-To"] = in_reply_to;
  }
  if (references && references.length > 0) {
    (resendPayload.headers as Record<string, string>)["References"] = references.join(" ");
  }

  try {
    const resendResponse = await fetch("https://api.resend.com/emails", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${resendApiKey}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(resendPayload),
    });

    if (!resendResponse.ok) {
      const errorData = await resendResponse.json().catch(() => ({}));
      return NextResponse.json(
        { error: (errorData as { message?: string }).message || "Failed to send email" },
        { status: 500 }
      );
    }
  } catch {
    return NextResponse.json(
      { error: "Failed to communicate with email service" },
      { status: 500 }
    );
  }

  // Determine thread: find existing if this is a reply, else create new
  let threadId: string;

  if (in_reply_to) {
    // Look for existing thread by matching in_reply_to against message_ids
    const existingEmail = await prisma.email.findFirst({
      where: {
        user_id: user.id,
        message_id: in_reply_to,
      },
      select: { thread_id: true },
    });

    if (existingEmail) {
      threadId = existingEmail.thread_id;

      // Update thread
      await prisma.thread.update({
        where: { id: threadId },
        data: {
          last_message_at: new Date(),
          message_count: { increment: 1 },
          participant_emails: JSON.stringify(
            Array.from(new Set([
              ...toAddresses,
              ...ccAddresses,
              user.email,
            ]))
          ),
        },
      });
    } else {
      // No matching thread found, create new
      const thread = await prisma.thread.create({
        data: {
          org_id: user.org_id,
          user_id: user.id,
          subject,
          participant_emails: JSON.stringify([...toAddresses, ...ccAddresses, user.email]),
          last_message_at: new Date(),
          message_count: 1,
          folder: "sent",
        },
      });
      threadId = thread.id;
    }
  } else {
    // New thread
    const thread = await prisma.thread.create({
      data: {
        org_id: user.org_id,
        user_id: user.id,
        subject,
        participant_emails: JSON.stringify([...toAddresses, ...ccAddresses, user.email]),
        last_message_at: new Date(),
        message_count: 1,
        folder: "sent",
      },
    });
    threadId = thread.id;
  }

  // Create Email record
  const email = await prisma.email.create({
    data: {
      org_id: user.org_id,
      thread_id: threadId,
      user_id: user.id,
      message_id: messageId,
      in_reply_to: in_reply_to || null,
      references_header: references ? references : Prisma.JsonNull,
      from_address: user.email,
      to_addresses: JSON.stringify(toAddresses),
      cc_addresses: JSON.stringify(ccAddresses),
      bcc_addresses: JSON.stringify(bccAddresses),
      subject,
      body_html: body_html || "",
      body_plain: body_plain || "",
      direction: "outbound",
      status: "sent",
      read: true,
      folder: "sent",
      received_at: new Date(),
    },
  });

  return NextResponse.json(
    {
      email: {
        id: email.id,
        thread_id: threadId,
        message_id: messageId,
        status: "sent",
      },
    },
    { status: 201 }
  );
}
