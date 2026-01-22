import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Inboxes.net",
  description: "Simple, affordable company email",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
