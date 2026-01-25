import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

/**
 * PATCH /api/aliases/:id
 * Updates an alias name and/or assigned users.
 * Admin-only endpoint.
 */
export async function PATCH(
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
    include: {
      alias_users: {
        include: {
          user: {
            select: {
              id: true,
              name: true,
              email: true,
            },
          },
        },
      },
    },
  });

  if (!alias) {
    return NextResponse.json({ error: "Alias not found" }, { status: 404 });
  }

  let body: { name?: string; users?: Array<{ user_id: string; can_send_as?: boolean }> };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { name, users } = body;

  // Update the alias name if provided
  if (name !== undefined) {
    if (typeof name !== "string" || name.trim() === "") {
      return NextResponse.json(
        { error: "name must be a non-empty string" },
        { status: 400 }
      );
    }

    await prisma.alias.update({
      where: { id },
      data: { name: name.trim() },
    });
  }

  // Update assigned users if provided
  if (users !== undefined) {
    if (!Array.isArray(users)) {
      return NextResponse.json(
        { error: "users must be an array" },
        { status: 400 }
      );
    }

    // Validate all user IDs exist in the org
    const userIds = users.map((u) => u.user_id);
    const orgUsers = await prisma.user.findMany({
      where: {
        id: { in: userIds },
        org_id: user.org_id,
      },
      select: { id: true },
    });

    const orgUserIds = new Set(orgUsers.map((u) => u.id));
    const invalidUserIds = userIds.filter((uid) => !orgUserIds.has(uid));

    if (invalidUserIds.length > 0) {
      return NextResponse.json(
        { error: `Invalid user IDs: ${invalidUserIds.join(", ")}` },
        { status: 400 }
      );
    }

    // Delete all existing alias_users and re-create with new list
    await prisma.aliasUser.deleteMany({
      where: { alias_id: id },
    });

    if (users.length > 0) {
      await prisma.aliasUser.createMany({
        data: users.map((u) => ({
          alias_id: id,
          user_id: u.user_id,
          can_send_as: u.can_send_as ?? true,
        })),
      });
    }
  }

  // Fetch updated alias
  const updatedAlias = await prisma.alias.findUnique({
    where: { id },
    include: {
      alias_users: {
        include: {
          user: {
            select: {
              id: true,
              name: true,
              email: true,
            },
          },
        },
      },
    },
  });

  return NextResponse.json({
    alias: {
      id: updatedAlias!.id,
      address: updatedAlias!.address,
      name: updatedAlias!.name,
      created_at: updatedAlias!.created_at,
      users: updatedAlias!.alias_users.map((au) => ({
        id: au.user.id,
        name: au.user.name,
        email: au.user.email,
        can_send_as: au.can_send_as,
      })),
    },
  });
}

/**
 * DELETE /api/aliases/:id
 * Deletes an alias and all its AliasUser records.
 * Admin-only endpoint.
 */
export async function DELETE(
  _request: NextRequest,
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

  // Delete all AliasUser records first (cascade not automatic in Prisma)
  await prisma.aliasUser.deleteMany({
    where: { alias_id: id },
  });

  // Delete the alias
  await prisma.alias.delete({
    where: { id },
  });

  return NextResponse.json({ success: true });
}
