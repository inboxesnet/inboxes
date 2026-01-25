import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/session";
import { prisma } from "@/lib/db";

export async function GET() {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Count threads with unread_count > 0 in inbox folder
  const unreadThreadCount = await prisma.thread.count({
    where: {
      user_id: user.id,
      folder: "inbox",
      unread_count: { gt: 0 },
      deleted_at: null,
    },
  });

  return NextResponse.json({ count: unreadThreadCount });
}
