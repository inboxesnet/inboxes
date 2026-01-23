import { NextRequest, NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;

  const thread = await prisma.thread.findFirst({
    where: {
      id,
      user_id: user.id,
      deleted_at: null,
    },
    include: {
      emails: {
        orderBy: { received_at: "asc" },
        select: {
          id: true,
          from_address: true,
          to_addresses: true,
          cc_addresses: true,
          subject: true,
          body_html: true,
          body_plain: true,
          direction: true,
          read: true,
          received_at: true,
          attachments: true,
        },
      },
    },
  });

  if (!thread) {
    return NextResponse.json({ error: "Thread not found" }, { status: 404 });
  }

  // Mark thread as read
  if (thread.unread_count > 0) {
    await Promise.all([
      prisma.thread.update({
        where: { id },
        data: { unread_count: 0 },
      }),
      prisma.email.updateMany({
        where: { thread_id: id, user_id: user.id, read: false },
        data: { read: true },
      }),
    ]);
  }

  return NextResponse.json({
    id: thread.id,
    subject: thread.subject,
    starred: thread.starred,
    folder: thread.folder,
    message_count: thread.message_count,
    emails: thread.emails.map((email) => ({
      id: email.id,
      from_address: email.from_address,
      to_addresses: email.to_addresses,
      cc_addresses: email.cc_addresses,
      subject: email.subject,
      body_html: email.body_html,
      body_plain: email.body_plain,
      direction: email.direction,
      read: email.read,
      received_at: email.received_at,
      attachments: email.attachments,
    })),
  });
}
