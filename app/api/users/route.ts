import { NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

export async function GET() {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

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
