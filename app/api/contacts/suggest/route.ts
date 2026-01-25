import { NextRequest, NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { getCurrentUser } from "@/lib/session";

interface ContactResult {
  email: string;
  name: string | null;
  frequency: number;
}

// Extract email address from "Name <email>" format or plain email
function extractEmail(address: string): string {
  const match = address.match(/<([^>]+)>/);
  return match ? match[1] : address;
}

// Extract name from "Name <email>" format
function extractName(address: string): string | null {
  const match = address.match(/^([^<]+)\s*</);
  return match ? match[1].trim() : null;
}

// GET /api/contacts/suggest?q=partial
// Returns matching email addresses from user's sent/received history
export async function GET(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const query = searchParams.get("q")?.toLowerCase().trim() || "";

  if (!query || query.length < 2) {
    return NextResponse.json({ contacts: [] });
  }

  // Get all emails for the user
  const emails = await prisma.email.findMany({
    where: { user_id: user.id },
    select: {
      from_address: true,
      to_addresses: true,
      cc_addresses: true,
      direction: true,
    },
  });

  // Build frequency map of all contacts
  const contactMap = new Map<string, { name: string | null; frequency: number }>();

  for (const email of emails) {
    // For inbound emails, extract from_address
    if (email.direction === "inbound") {
      const fromEmail = extractEmail(email.from_address).toLowerCase();
      const fromName = extractName(email.from_address);
      const existing = contactMap.get(fromEmail);
      if (existing) {
        existing.frequency++;
        // Prefer a name over null
        if (fromName && !existing.name) {
          existing.name = fromName;
        }
      } else {
        contactMap.set(fromEmail, { name: fromName, frequency: 1 });
      }
    }

    // For outbound emails, extract to_addresses and cc_addresses
    if (email.direction === "outbound") {
      const toAddresses = email.to_addresses as string[];
      const ccAddresses = email.cc_addresses as string[];

      const allAddresses = [...toAddresses, ...ccAddresses];
      for (const address of allAddresses) {
        const emailAddr = extractEmail(address).toLowerCase();
        const name = extractName(address);
        const existing = contactMap.get(emailAddr);
        if (existing) {
          existing.frequency++;
          if (name && !existing.name) {
            existing.name = name;
          }
        } else {
          contactMap.set(emailAddr, { name, frequency: 1 });
        }
      }
    }
  }

  // Filter contacts matching the query (matches email or name)
  const matchingContacts: ContactResult[] = [];

  contactMap.forEach((data, email) => {
    const emailMatches = email.includes(query);
    const nameMatches = data.name?.toLowerCase().includes(query);

    if (emailMatches || nameMatches) {
      matchingContacts.push({
        email,
        name: data.name,
        frequency: data.frequency,
      });
    }
  });

  // Sort by frequency (descending) and take top 10
  matchingContacts.sort((a, b) => b.frequency - a.frequency);
  const topContacts = matchingContacts.slice(0, 10);

  return NextResponse.json({ contacts: topContacts });
}
