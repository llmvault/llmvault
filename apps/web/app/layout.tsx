import type { Metadata } from "next"
import { Bricolage_Grotesque } from "next/font/google"
import "./hero.css"
// import { QueryProvider } from "@/components/query-provider"
// import { TooltipProvider } from "@/components/ui/tooltip"
// import { Toaster } from "@/components/ui/sonner"

const bricolage = Bricolage_Grotesque({})
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
    <html lang="en" className="bg-background font-sans antialiased">
      <body
        className={bricolage.className}
        // style={{ "--font-bricolage": bricolage.style.fontFamily }}
      >
        {children}
        {/* <QueryProvider>
          <TooltipProvider>{children}</TooltipProvider>
          <Toaster position="top-center" />
        </QueryProvider> */}
      </body>
    </html>
  )
}
