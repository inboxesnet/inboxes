import { NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { getCurrentUser } from "@/lib/session";

export async function GET() {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  // Get all aliases the user can send as (can_send_as = true)
  const aliasUsers = await prisma.aliasUser.findMany({
    where: {
      user_id: user.id,
      can_send_as: true,
    },
    include: {
      alias: {
        select: {
          id: true,
          address: true,
          name: true,
        },
      },
    },
  });

  const aliases = aliasUsers.map((au) => ({
    id: au.alias.id,
    address: au.alias.address,
    name: au.alias.name,
  }));

  return NextResponse.json({
    personal_email: user.email,
    aliases,
  });
}
