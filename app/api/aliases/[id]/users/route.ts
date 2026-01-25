import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

/**
 * POST /api/aliases/:id/users
 * Adds a user to an alias.
 * Admin-only endpoint.
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  const { id } = await params;

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

  let body: { user_id?: string; can_send_as?: boolean };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { user_id, can_send_as } = body;

  if (!user_id || typeof user_id !== "string") {
    return NextResponse.json({ error: "user_id is required" }, { status: 400 });
  }

  // Verify the user exists in the same org
  const targetUser = await prisma.user.findFirst({
    where: {
      id: user_id,
      org_id: user.org_id,
    },
  });

  if (!targetUser) {
    return NextResponse.json({ error: "User not found in organization" }, { status: 404 });
  }

  // Check if user is already assigned to this alias
  const existingAliasUser = await prisma.aliasUser.findUnique({
    where: {
      alias_id_user_id: {
        alias_id: id,
        user_id: user_id,
      },
    },
  });

  if (existingAliasUser) {
    return NextResponse.json(
      { error: "User is already assigned to this alias" },
      { status: 409 }
    );
  }

  // Create the alias user assignment
  const aliasUser = await prisma.aliasUser.create({
    data: {
      alias_id: id,
      user_id: user_id,
      can_send_as: can_send_as ?? true,
    },
    include: {
      user: {
        select: {
          id: true,
          name: true,
          email: true,
        },
      },
    },
  });

  return NextResponse.json(
    {
      alias_user: {
        id: aliasUser.user.id,
        name: aliasUser.user.name,
        email: aliasUser.user.email,
        can_send_as: aliasUser.can_send_as,
      },
    },
    { status: 201 }
  );
}
