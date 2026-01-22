import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { getCurrentUser } from "@/lib/session";

const DOMAIN_REGEX = /^(?!-)([a-zA-Z0-9-]{1,63}(?<!-)\.)+[a-zA-Z]{2,}$/;

export async function POST(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (user.role !== "admin") {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }

  let body: { domain?: string };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { domain } = body;

  if (!domain || typeof domain !== "string") {
    return NextResponse.json({ error: "domain is required" }, { status: 400 });
  }

  const cleanDomain = domain.trim().toLowerCase();

  // Validate domain format
  if (!DOMAIN_REGEX.test(cleanDomain)) {
    return NextResponse.json(
      { error: "Invalid domain format. Provide a valid domain (e.g., example.com)" },
      { status: 400 }
    );
  }

  // Check if domain already registered
  const existingDomain = await prisma.domain.findUnique({
    where: { domain: cleanDomain },
  });
  if (existingDomain) {
    return NextResponse.json({ error: "Domain already registered" }, { status: 409 });
  }

  // Register domain with Resend API
  const resendApiKey = process.env.RESEND_API_KEY;
  if (!resendApiKey) {
    return NextResponse.json(
      { error: "Email service not configured" },
      { status: 500 }
    );
  }

  let resendDomain: {
    id: string;
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
    const resendResponse = await fetch("https://api.resend.com/domains", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${resendApiKey}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ name: cleanDomain }),
    });

    if (!resendResponse.ok) {
      const errorData = await resendResponse.json().catch(() => ({}));
      return NextResponse.json(
        { error: (errorData as { message?: string }).message || "Failed to register domain with email service" },
        { status: resendResponse.status === 409 ? 409 : 500 }
      );
    }

    resendDomain = await resendResponse.json();
  } catch {
    return NextResponse.json(
      { error: "Failed to communicate with email service" },
      { status: 500 }
    );
  }

  // Create Domain record in database
  const domainRecord = await prisma.domain.create({
    data: {
      org_id: user.org_id,
      domain: cleanDomain,
      status: "pending",
    },
  });

  // Format DNS records for response
  const dnsRecords = resendDomain.records.map((record) => ({
    type: record.type,
    name: record.name,
    value: record.value,
    priority: record.priority ?? null,
    status: record.status,
  }));

  return NextResponse.json(
    {
      domain: {
        id: domainRecord.id,
        domain: domainRecord.domain,
        status: domainRecord.status,
      },
      dns_records: dnsRecords,
    },
    { status: 201 }
  );
}
