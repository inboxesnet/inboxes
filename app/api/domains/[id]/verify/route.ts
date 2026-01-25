import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { requireAdmin } from "@/lib/session";

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const result = await requireAdmin();
  if ("error" in result) return result.error;
  const user = result;

  const { id } = await params;

  // Find domain belonging to user's org
  const domain = await prisma.domain.findFirst({
    where: { id, org_id: user.org_id },
  });

  if (!domain) {
    return NextResponse.json({ error: "Domain not found" }, { status: 404 });
  }

  const resendApiKey = process.env.RESEND_API_KEY;
  if (!resendApiKey) {
    return NextResponse.json(
      { error: "Email service not configured" },
      { status: 500 }
    );
  }

  let resendDomain: {
    id: string;
    status: string;
    records: Array<{
      record: string;
      name: string;
      type: string;
      ttl: string;
      status: string;
      value: string;
      priority?: number;
    }>;
  };

  try {
    // Trigger Resend domain verification check
    await fetch(
      `https://api.resend.com/domains/${domain.domain}/verify`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${resendApiKey}`,
        },
      }
    );

    // Get current domain status from Resend
    const statusResponse = await fetch(
      `https://api.resend.com/domains/${domain.domain}`,
      {
        method: "GET",
        headers: {
          Authorization: `Bearer ${resendApiKey}`,
        },
      }
    );

    if (!statusResponse.ok) {
      return NextResponse.json(
        { error: "Failed to check domain verification status" },
        { status: 500 }
      );
    }

    resendDomain = await statusResponse.json();
  } catch {
    return NextResponse.json(
      { error: "Failed to communicate with email service" },
      { status: 500 }
    );
  }

  // Determine verification status for each record type
  const records = resendDomain.records || [];

  const mxVerified = records.some(
    (r) => r.type.toUpperCase() === "MX" && r.status === "verified"
  );

  const spfVerified = records.some(
    (r) =>
      r.type.toUpperCase() === "TXT" &&
      (r.name.includes("_spf") || (r.value && r.value.includes("spf"))) &&
      r.status === "verified"
  );

  const dkimVerified = records.some(
    (r) =>
      (r.type.toUpperCase() === "CNAME" || r.type.toUpperCase() === "TXT") &&
      r.name.includes("dkim") &&
      r.status === "verified"
  );

  const allVerified = mxVerified && spfVerified && dkimVerified;

  // Determine new status
  let newStatus: "pending" | "verified" | "active" = domain.status as "pending" | "verified" | "active";
  if (allVerified) {
    newStatus = "active";
  } else if (mxVerified || spfVerified || dkimVerified) {
    newStatus = "pending";
  }

  const verifiedAt = allVerified && !domain.verified_at ? new Date() : domain.verified_at;

  // Update domain record
  const updatedDomain = await prisma.domain.update({
    where: { id: domain.id },
    data: {
      mx_verified: mxVerified,
      spf_verified: spfVerified,
      dkim_verified: dkimVerified,
      status: newStatus,
      verified_at: verifiedAt,
    },
  });

  return NextResponse.json({
    domain: {
      id: updatedDomain.id,
      domain: updatedDomain.domain,
      status: updatedDomain.status,
      mx_verified: updatedDomain.mx_verified,
      spf_verified: updatedDomain.spf_verified,
      dkim_verified: updatedDomain.dkim_verified,
      verified_at: updatedDomain.verified_at,
    },
    records: records.map((r) => ({
      type: r.type,
      name: r.name,
      value: r.value,
      priority: r.priority ?? null,
      status: r.status,
    })),
  });
}
