import { NextRequest, NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";

export async function PATCH(
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
  });

  if (!thread) {
    return NextResponse.json({ error: "Thread not found" }, { status: 404 });
  }

  // Toggle starred status
  const updatedThread = await prisma.thread.update({
    where: { id },
    data: { starred: !thread.starred },
  });

  return NextResponse.json({
    id: updatedThread.id,
    subject: updatedThread.subject,
    folder: updatedThread.folder,
    starred: updatedThread.starred,
    unread_count: updatedThread.unread_count,
    message_count: updatedThread.message_count,
    last_message_at: updatedThread.last_message_at,
  });
}
