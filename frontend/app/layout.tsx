import type { Metadata } from "next";
import { JetBrains_Mono, Space_Grotesk } from "next/font/google";
import type { ReactNode } from "react";
import { Providers } from "@/components/providers";
import { Sidebar } from "@/components/ui/sidebar";
import "./globals.css";

const spaceGrotesk = Space_Grotesk({
  subsets: ["latin"],
  variable: "--font-ui",
  display: "swap",
});

const jetBrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: {
    default: "Lexos",
    template: "%s | Lexos",
  },
  description: "Cloud-native transcription, summarization, and RAG document processing.",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en" className={`${spaceGrotesk.variable} ${jetBrainsMono.variable}`}>
      <body>
        <Providers>
          <Sidebar />
          <main className="min-h-screen pl-20 lg:pl-72">
            <div className="mx-auto w-full max-w-[1600px] p-5 md:p-8 lg:p-12">{children}</div>
          </main>
        </Providers>
      </body>
    </html>
  );
}
