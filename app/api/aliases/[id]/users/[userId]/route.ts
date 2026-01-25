import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

/**
 * DELETE /api/aliases/:id/users/:userId
 * Removes a user from an alias.
 * Admin-only endpoint.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string; userId: string }> }
) {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  const { id, userId } = await params;

  // Find the alias and verify it belongs to the user's org
  const alias = await prisma.alias.findFirst({
    where: {
      id,
      org_id: user.org_id,
    },
  });

  if (!alias) {
    return NextResponse.json({ error: "Alias not found" }, { status: 404 });
  }

  // Check if the alias user assignment exists
  const aliasUser = await prisma.aliasUser.findUnique({
    where: {
      alias_id_user_id: {
        alias_id: id,
        user_id: userId,
      },
    },
  });

  if (!aliasUser) {
    return NextResponse.json(
      { error: "User is not assigned to this alias" },
      { status: 404 }
    );
  }

  // Delete the alias user assignment
  await prisma.aliasUser.delete({
    where: {
      alias_id_user_id: {
        alias_id: id,
        user_id: userId,
      },
    },
  });

  return NextResponse.json({ success: true });
}
