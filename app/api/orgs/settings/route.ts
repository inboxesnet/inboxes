import { NextRequest, NextResponse } from "next/server";
import { requireAdmin } from "@/lib/session";
import { prisma } from "@/lib/db";

export async function GET() {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  const org = await prisma.org.findUnique({
    where: { id: user.org_id },
    select: {
      id: true,
      name: true,
      catch_all_enabled: true,
    },
  });

  if (!org) {
    return NextResponse.json({ error: "Organization not found" }, { status: 404 });
  }

  return NextResponse.json({ org });
}

export async function PATCH(request: NextRequest) {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  let body;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { catch_all_enabled } = body;

  if (typeof catch_all_enabled !== "boolean") {
    return NextResponse.json(
      { error: "catch_all_enabled must be a boolean" },
      { status: 400 }
    );
  }

  const org = await prisma.org.update({
    where: { id: user.org_id },
    data: { catch_all_enabled },
    select: {
      id: true,
      name: true,
      catch_all_enabled: true,
    },
  });

  return NextResponse.json({ org });
}
