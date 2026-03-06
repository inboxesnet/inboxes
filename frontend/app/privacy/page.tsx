import Link from "next/link";
import { ThemeToggle } from "@/components/theme-toggle";

export const metadata = {
  title: "Privacy Policy - Inboxes",
};

export default function PrivacyPage() {
  return (
    <main className="min-h-screen bg-background">
      <nav className="flex items-center justify-between px-6 py-4 max-w-3xl mx-auto">
        <Link href="/" className="flex items-center gap-2">
          <span className="text-xl font-bold">Inboxes</span>
        </Link>
        <ThemeToggle />
      </nav>

      <article className="px-6 py-12 max-w-3xl mx-auto prose prose-neutral dark:prose-invert prose-sm">
        <h1>Privacy Policy</h1>

        <p>
          Inboxes is open source software released under the{" "}
          <a href="https://github.com/headswim/inboxes/blob/master/LICENSE" target="_blank" rel="noopener noreferrer">
            MIT License
          </a>.
        </p>

        <p>
          When self-hosted, all data stays on your server. Inboxes does not phone home,
          collect telemetry, or send data to any third party. You control your data entirely.
        </p>

        <p>
          If you are using a commercially hosted version of Inboxes, the hosting provider's
          privacy policy applies. See your hosting provider for details.
        </p>
      </article>
    </main>
  );
}
