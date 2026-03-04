import Link from "next/link";
import Image from "next/image";
import { Mail, Key, Globe, Zap, Shield, Users, ArrowRight } from "lucide-react";
import { ThemeToggle } from "@/components/theme-toggle";

export function MarketingPage() {
  return (
    <main className="min-h-screen bg-gradient-to-b from-background to-muted/50">
      {/* Nav */}
      <nav className="flex items-center justify-between px-6 py-4 max-w-6xl mx-auto">
        <div className="flex items-center gap-2">
          <Image src="/icon.svg" alt="Inboxes" width={32} height={32} className="rounded-lg" />
          <span className="text-xl font-bold">Inboxes</span>
        </div>
        <div className="flex items-center gap-3">
          <ThemeToggle />
          <Link
            href="/login"
            className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors px-3 py-2"
          >
            Log in
          </Link>
          <Link
            href="/signup"
            className="text-sm font-medium bg-primary text-primary-foreground px-4 py-2 rounded-lg hover:opacity-90 transition-opacity"
          >
            Get Started
          </Link>
        </div>
      </nav>

      {/* Hero */}
      <section className="px-6 pt-20 pb-16 max-w-4xl mx-auto text-center">
        <h1 className="text-5xl sm:text-6xl font-bold tracking-tight text-foreground">
          The missing inbox
          <br />
          <span className="text-transparent bg-clip-text bg-gradient-to-r from-blue-600 to-indigo-600">
            for Resend
          </span>
        </h1>
        <p className="mt-6 text-lg sm:text-xl text-muted-foreground max-w-2xl mx-auto leading-relaxed">
          All your Resend emails in one place. Bring your own API key,
          import your full history, and get a real inbox for every domain.
        </p>
        <div className="mt-10 flex flex-col sm:flex-row items-center justify-center gap-4">
          <Link
            href="/signup"
            className="flex items-center gap-2 bg-primary text-primary-foreground px-8 py-3 rounded-lg text-lg font-medium hover:opacity-90 transition-opacity"
          >
            Connect your Resend
            <ArrowRight className="h-5 w-5" />
          </Link>
          <p className="text-sm text-muted-foreground">
            Paste your API key. Done in 2 minutes.
          </p>
        </div>
      </section>

      {/* Features */}
      <section className="px-6 py-20 max-w-5xl mx-auto">
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-8">
          <FeatureCard
            icon={<Key className="h-5 w-5" />}
            title="Bring your own key"
            description="Your Resend API key, your infrastructure. We never touch your DNS or sending reputation."
          />
          <FeatureCard
            icon={<Globe className="h-5 w-5" />}
            title="Inbox per domain"
            description="Discord-style sidebar. Switch between domains instantly. Each one gets its own inbox."
          />
          <FeatureCard
            icon={<Mail className="h-5 w-5" />}
            title="Full email history"
            description="Import every sent and received email from Resend. Threaded automatically."
          />
          <FeatureCard
            icon={<Zap className="h-5 w-5" />}
            title="Real-time delivery"
            description="New emails arrive instantly via webhooks. No refresh, no polling."
          />
          <FeatureCard
            icon={<Users className="h-5 w-5" />}
            title="Team-ready"
            description="Invite teammates, set up shared aliases like support@ or hello@, manage roles."
          />
          <FeatureCard
            icon={<Shield className="h-5 w-5" />}
            title="Spam filtering"
            description="Built-in spam scoring based on SPF, DKIM, and header analysis. Bad emails stay out."
          />
        </div>
      </section>

      {/* CTA */}
      <section className="px-6 py-20 max-w-4xl mx-auto text-center">
        <div className="bg-primary rounded-2xl px-8 py-14 sm:px-14">
          <h2 className="text-3xl font-bold text-primary-foreground">
            Resend sends your email.
            <br />
            We give you the inbox.
          </h2>
          <p className="mt-3 text-primary-foreground/70 text-lg">
            Paste your API key and start reading in minutes.
          </p>
          <Link
            href="/signup"
            className="mt-8 inline-flex items-center gap-2 bg-primary-foreground text-primary px-8 py-3 rounded-lg text-lg font-medium hover:opacity-90 transition-opacity"
          >
            Get started free
            <ArrowRight className="h-5 w-5" />
          </Link>
        </div>
      </section>

      {/* Footer */}
      <footer className="px-6 py-8 max-w-6xl mx-auto border-t">
        <div className="flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Image src="/icon.svg" alt="Inboxes" width={24} height={24} className="rounded" />
            Inboxes
          </div>
          <div className="flex items-center gap-6 text-sm text-muted-foreground">
            <Link href="/terms" className="hover:text-foreground transition-colors">
              Terms
            </Link>
            <Link href="/privacy" className="hover:text-foreground transition-colors">
              Privacy
            </Link>
            <Link href="/login" className="hover:text-foreground transition-colors">
              Log in
            </Link>
            <Link href="/signup" className="hover:text-foreground transition-colors">
              Sign up
            </Link>
          </div>
        </div>
      </footer>
    </main>
  );
}

function FeatureCard({
  icon,
  title,
  description,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
}) {
  return (
    <div className="rounded-xl border bg-card p-6 hover:shadow-md transition-shadow">
      <div className="h-10 w-10 rounded-lg bg-secondary flex items-center justify-center text-foreground">
        {icon}
      </div>
      <h3 className="mt-4 font-semibold text-foreground">{title}</h3>
      <p className="mt-2 text-sm text-muted-foreground leading-relaxed">
        {description}
      </p>
    </div>
  );
}
