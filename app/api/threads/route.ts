import { NextRequest, NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";

export async function GET(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = request.nextUrl;
  const folder = searchParams.get("folder") || "inbox";
  const page = parseInt(searchParams.get("page") || "1", 10);
  const limit = 20;
  const skip = (page - 1) * limit;

  const where = {
    user_id: user.id,
    folder: folder as "inbox" | "sent" | "archive" | "trash",
    deleted_at: null,
  };

  const [threads, total] = await Promise.all([
    prisma.thread.findMany({
      where,
      orderBy: { last_message_at: "desc" },
      skip,
      take: limit,
      include: {
        emails: {
          orderBy: { received_at: "desc" },
          take: 1,
          select: {
            id: true,
            from_address: true,
            to_addresses: true,
            body_plain: true,
            received_at: true,
            direction: true,
            original_to: true,
          },
        },
      },
    }),
    prisma.thread.count({ where }),
  ]);

  const formattedThreads = threads.map((thread) => {
    const latestEmail = thread.emails[0];
    const preview = latestEmail?.body_plain
      ? latestEmail.body_plain.substring(0, 120)
      : "";

    return {
      id: thread.id,
      subject: thread.subject,
      unread_count: thread.unread_count,
      starred: thread.starred,
      message_count: thread.message_count,
      last_message_at: thread.last_message_at,
      from_address: latestEmail?.from_address || "",
      to_addresses: latestEmail?.to_addresses || [],
      preview,
      original_to: latestEmail?.original_to || null,
    };
  });

  return NextResponse.json({
    threads: formattedThreads,
    total,
    page,
    totalPages: Math.ceil(total / limit),
  });
}
