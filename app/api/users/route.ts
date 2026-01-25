import { NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { getCurrentUser } from "@/lib/session";

export async function GET() {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Admin-only endpoint
  if (user.role !== "admin") {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  // Get all users in the org
  const users = await prisma.user.findMany({
    where: { org_id: user.org_id },
    select: {
      id: true,
      email: true,
      name: true,
      role: true,
      status: true,
      invite_expires_at: true,
      claimed_at: true,
      created_at: true,
    },
    orderBy: { created_at: "asc" },
  });

  return NextResponse.json({ users });
}
