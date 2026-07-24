import type { Metadata } from "next"
import { Bricolage_Grotesque, Geist_Mono } from "next/font/google"
import "./hero.css"
import { Toast } from "@heroui/react"
import { QueryProvider } from "@/components/query-provider"
import { ThemeProviders } from "@/components/theme-providers"

const bricolageSans = Bricolage_Grotesque({
  subsets: ["latin"],
  variable: "--font-bricolage-sans",
})
const geistMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-geist-mono",
})
export const metadata: Metadata = {
  title: {
    default: "Hivy",
    template: "%s — Hivy",
  },
  description:
    "Hire AI employees for your team. Autonomous agents that learn, understand your organization, and take initiative.",
  icons: {
    icon: [
      { url: "/favicon.ico", sizes: "48x48" },
      { url: "/favicon-32x32.png", sizes: "32x32", type: "image/png" },
      { url: "/favicon-16x16.png", sizes: "16x16", type: "image/png" },
    ],
    apple: [{ url: "/apple-touch-icon.png", sizes: "180x180" }],
  },
  manifest: "/site.webmanifest",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html
      lang="en"
      className={`${bricolageSans.variable} ${geistMono.variable} dark font-sans antialiased`}
      suppressHydrationWarning
    >
      <body className={bricolageSans.className}>
        <ThemeProviders>
          <QueryProvider>
            {children}
            <Toast.Provider placement="top" />
          </QueryProvider>
        </ThemeProviders>
      </body>
    </html>
  )
}
