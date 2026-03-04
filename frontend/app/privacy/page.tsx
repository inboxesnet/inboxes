import Link from "next/link";
import Image from "next/image";
import { ThemeToggle } from "@/components/theme-toggle";

export const metadata = {
  title: "Privacy Policy — Inboxes",
};

export default function PrivacyPage() {
  return (
    <main className="min-h-screen bg-background">
      <nav className="flex items-center justify-between px-6 py-4 max-w-3xl mx-auto">
        <Link href="/" className="flex items-center gap-2">
          <Image src="/icon.svg" alt="Inboxes" width={32} height={32} className="rounded-lg" />
          <span className="text-xl font-bold">Inboxes</span>
        </Link>
        <ThemeToggle />
      </nav>

      <article className="px-6 py-12 max-w-3xl mx-auto prose prose-neutral dark:prose-invert prose-sm">
        <h1>Privacy Policy</h1>
        <p className="text-muted-foreground">Last updated: March 4, 2026</p>

        <h2>What We Collect</h2>
        <p>When you use Inboxes, we store:</p>
        <ul>
          <li>Your account information (name, email, hashed password)</li>
          <li>Your Resend API key (encrypted with AES-256-GCM)</li>
          <li>Emails synced from your Resend account (subject, body, headers, attachments, metadata)</li>
          <li>Actions you take in the app (thread labels, read status, drafts, settings)</li>
        </ul>

        <h2>How We Use It</h2>
        <p>
          We use your data to provide the Service. That's it. We don't run analytics on your email
          content, we don't build advertising profiles, and we don't sell your data.
        </p>

        <h2>Cookies</h2>
        <p>
          We use a single httpOnly cookie containing a JWT for authentication. No tracking cookies,
          no third-party cookies, no cookie banners.
        </p>

        <h2>Third-Party Services</h2>
        <p>We use the following third-party services:</p>
        <ul>
          <li><strong>Resend</strong> — to send and receive email (using your API key)</li>
          <li><strong>Stripe</strong> — to process payments (if on a paid plan)</li>
        </ul>
        <p>
          These services have their own privacy policies. We send them only what's necessary
          to provide their function.
        </p>

        <h2>Data Storage</h2>
        <p>
          Your data is stored on servers in the United States. Emails, attachments, and API keys
          are stored in PostgreSQL. Session data uses Redis. We use encryption at rest for
          sensitive fields.
        </p>

        <h2>Data Deletion</h2>
        <p>
          Delete your organization from Settings and all associated data is removed. Trashed
          emails are automatically purged after 30 days. If you want your account fully removed
          and can't do it yourself, email us.
        </p>

        <h2>Security</h2>
        <p>
          We use HTTPS, httpOnly cookies, JWT token blacklisting, rate limiting, input validation,
          and AES-256-GCM encryption for API keys. Passwords are hashed with bcrypt.
        </p>

        <h2>Children</h2>
        <p>
          The Service is not intended for children under 13. We don't knowingly collect data from
          children.
        </p>

        <h2>Changes</h2>
        <p>
          We may update this policy. Continued use after changes constitutes acceptance.
        </p>

        <h2>Contact</h2>
        <p>
          Questions about your data? Email us at the address listed on our website.
        </p>
      </article>
    </main>
  );
}
