import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "PR Blast Radius Analyzer",
  description: "Visualize the impact of code changes with static analysis + LLM visualization",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className="antialiased bg-gray-50 dark:bg-gray-900">
        {children}
      </body>
    </html>
  );
}
