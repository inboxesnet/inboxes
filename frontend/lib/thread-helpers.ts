export function extractDisplayName(raw: string): string {
  if (!raw) return "";
  // Parse "Display Name <email@example.com>" format
  const ltIndex = raw.indexOf("<");
  if (ltIndex > 0) {
    const name = raw.substring(0, ltIndex).trim().replace(/^["']|["']$/g, "");
    if (name) return name;
  }
  // Fallback: local part of email
  const atIndex = raw.indexOf("@");
  return atIndex > 0 ? raw.substring(0, atIndex) : raw;
}

export function extractSender(emails: string[]): string {
  if (!emails || emails.length === 0) return "Unknown";
  return extractDisplayName(emails[0]);
}

export function decodeHtmlEntities(text: string): string {
  return text
    .replace(/&#x([0-9a-fA-F]+);/g, (_, hex) =>
      String.fromCharCode(parseInt(hex, 16))
    )
    .replace(/&#(\d+);/g, (_, dec) =>
      String.fromCharCode(parseInt(dec, 10))
    )
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'");
}

export function cleanSnippet(text: string): string {
  return decodeHtmlEntities(text)
    .replace(/\s+/g, " ")
    .trim();
}

export function parseParticipants(raw: string[] | string): string[] {
  if (Array.isArray(raw)) return raw;
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw);
    } catch {
      return [];
    }
  }
  return [];
}
