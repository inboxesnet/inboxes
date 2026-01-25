import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

/**
 * GET /api/aliases
 * Returns all aliases for the current org with assigned users.
 * Admin-only endpoint.
 */
export async function GET() {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  const aliases = await prisma.alias.findMany({
    where: { org_id: user.org_id },
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
    orderBy: { created_at: "desc" },
  });

  return NextResponse.json({
    aliases: aliases.map((alias) => ({
      id: alias.id,
      address: alias.address,
      name: alias.name,
      created_at: alias.created_at,
      users: alias.alias_users.map((au) => ({
        id: au.user.id,
        name: au.user.name,
        email: au.user.email,
        can_send_as: au.can_send_as,
      })),
    })),
  });
}

/**
 * POST /api/aliases
 * Creates a new alias for the current org.
 * Admin-only endpoint.
 * Validates that the address matches the org's active domain.
 */
export async function POST(request: NextRequest) {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  let body: { address?: string; name?: string };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { address, name } = body;

  if (!address || typeof address !== "string") {
    return NextResponse.json({ error: "address is required" }, { status: 400 });
  }

  if (!name || typeof name !== "string") {
    return NextResponse.json({ error: "name is required" }, { status: 400 });
  }

  const cleanAddress = address.trim().toLowerCase();
  const cleanName = name.trim();

  // Validate email address format
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!emailRegex.test(cleanAddress)) {
    return NextResponse.json(
      { error: "Invalid email address format" },
      { status: 400 }
    );
  }

  // Extract domain from address
  const addressDomain = cleanAddress.split("@")[1];

  // Check that org has an active domain matching the address domain
  const orgDomain = await prisma.domain.findFirst({
    where: {
      org_id: user.org_id,
      domain: addressDomain,
      status: "active",
    },
  });

  if (!orgDomain) {
    return NextResponse.json(
      { error: "Address must use your organization's verified domain" },
      { status: 400 }
    );
  }

  // Check if alias address already exists
  const existingAlias = await prisma.alias.findUnique({
    where: { address: cleanAddress },
  });

  if (existingAlias) {
    return NextResponse.json(
      { error: "Alias address already exists" },
      { status: 409 }
    );
  }

  // Create the alias
  const alias = await prisma.alias.create({
    data: {
      org_id: user.org_id,
      address: cleanAddress,
      name: cleanName,
    },
  });

  return NextResponse.json(
    {
      alias: {
        id: alias.id,
        address: alias.address,
        name: alias.name,
        created_at: alias.created_at,
        users: [],
      },
    },
    { status: 201 }
  );
}
