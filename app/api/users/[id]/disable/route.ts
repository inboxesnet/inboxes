import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  const { id } = await params;

  // Cannot disable yourself
  if (id === user.id) {
    return NextResponse.json(
      { error: "You cannot disable your own account" },
      { status: 400 }
    );
  }

  // Find the user to disable
  const targetUser = await prisma.user.findFirst({
    where: {
      id,
      org_id: user.org_id,
    },
  });

  if (!targetUser) {
    return NextResponse.json({ error: "User not found" }, { status: 404 });
  }

  // Can only disable active users
  if (targetUser.status !== "active") {
    return NextResponse.json(
      { error: "Can only disable active users" },
      { status: 400 }
    );
  }

  // Disable the user
  const updatedUser = await prisma.user.update({
    where: { id: targetUser.id },
    data: {
      status: "disabled",
    },
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
  });

  return NextResponse.json({ user: updatedUser });
}
