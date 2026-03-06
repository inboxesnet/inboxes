import Link from "next/link";
import { ThemeToggle } from "@/components/theme-toggle";

export const metadata = {
  title: "Terms and Conditions - Inboxes",
};

export default function TermsPage() {
  return (
    <main className="min-h-screen bg-background">
      <nav className="flex items-center justify-between px-6 py-4 max-w-3xl mx-auto">
        <Link href="/" className="flex items-center gap-2">
          <span className="text-xl font-bold">Inboxes</span>
        </Link>
        <ThemeToggle />
      </nav>

      <article className="px-6 py-12 max-w-3xl mx-auto prose prose-neutral dark:prose-invert prose-sm">
        <h1>Terms and Conditions</h1>

        <p>
          Inboxes is open source software released under the{" "}
          <a href="https://github.com/headswim/inboxes/blob/master/LICENSE" target="_blank" rel="noopener noreferrer">
            MIT License
          </a>.
        </p>

        <p>
          This software is provided "as is", without warranty of any kind, express or implied.
          See the LICENSE file for the full terms.
        </p>

        <p>
          If you are using a commercially hosted version of Inboxes, additional terms may apply.
          See your hosting provider for details.
        </p>
      </article>
    </main>
  );
}
