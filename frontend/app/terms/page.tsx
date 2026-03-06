import Link from "next/link";
import Image from "next/image";
import { ThemeToggle } from "@/components/theme-toggle";

export const metadata = {
  title: "Terms of Service - Inboxes",
};

export default function TermsPage() {
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
        <h1>Terms of Service</h1>
        <p className="text-muted-foreground">Last updated: March 4, 2026</p>

        <h2>1. Acceptance</h2>
        <p>
          By using Inboxes ("the Service"), you agree to these terms. If you don't agree, don't use it.
        </p>

        <h2>2. What We Do</h2>
        <p>
          Inboxes is an email client that connects to your Resend account using an API key you provide.
          We store your emails, metadata, and account information to provide the Service.
        </p>

        <h2>3. Your Account</h2>
        <p>
          You're responsible for your account credentials and everything that happens under your account.
          Keep your password secure. If you suspect unauthorized access, change your password immediately.
        </p>

        <h2>4. Your Data</h2>
        <p>
          You own your data. We store it to provide the Service. You can delete your organization at
          any time, which removes all associated data. We don't sell your data to third parties.
        </p>

        <h2>5. Your Resend API Key</h2>
        <p>
          You provide your own Resend API key. We encrypt it at rest (AES-256-GCM) and use it solely
          to interact with the Resend API on your behalf. We are not responsible for your Resend
          account, billing, or sending reputation.
        </p>

        <h2>6. Acceptable Use</h2>
        <p>Don't use the Service to:</p>
        <ul>
          <li>Send spam or unsolicited bulk email</li>
          <li>Violate any applicable law</li>
          <li>Interfere with or disrupt the Service</li>
          <li>Attempt to gain unauthorized access to other accounts</li>
        </ul>
        <p>We may suspend or terminate accounts that violate these terms.</p>

        <h2>7. Paid Plans</h2>
        <p>
          If you subscribe to a paid plan, billing is handled through Stripe. Subscriptions renew
          automatically. You can cancel anytime through the billing portal. Refunds are handled
          on a case-by-case basis.
        </p>

        <h2>8. Availability</h2>
        <p>
          We try to keep the Service running but don't guarantee 100% uptime.
          We're not liable for downtime, data loss, or issues caused by Resend, your DNS
          configuration, or third-party services.
        </p>

        <h2>9. Limitation of Liability</h2>
        <p>
          The Service is provided "as is" without warranties of any kind. To the maximum extent
          permitted by law, we are not liable for any indirect, incidental, or consequential damages
          arising from your use of the Service.
        </p>

        <h2>10. Changes</h2>
        <p>
          We may update these terms. Continued use of the Service after changes constitutes acceptance.
          We'll make reasonable efforts to notify you of significant changes.
        </p>

        <h2>11. Governing Law</h2>
        <p>
          These terms are governed by the laws of the United States. Any disputes will be resolved
          in U.S. courts.
        </p>

        <h2>12. Contact</h2>
        <p>
          Questions? Email us at the address listed on our website.
        </p>
      </article>
    </main>
  );
}
