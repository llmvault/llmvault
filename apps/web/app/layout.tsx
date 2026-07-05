import type { Metadata } from "next"
import { Bricolage_Grotesque, Geist_Mono } from "next/font/google"
import "./hero.css"
import { Toast } from "@heroui/react"
import { PublicConfigScript } from "@/components/public-config-script"
import { QueryProvider } from "@/components/query-provider"
import { ThemeProviders } from "@/components/theme-providers"

// Applies the saved theme preset before first paint to avoid a flash.
// next-themes injects its own equivalent script for the light/dark mode.
const PRESET_NO_FLASH = `(function(){try{var p=localStorage.getItem('hivy-theme-preset');if(p&&p!=='default')document.documentElement.dataset.themePreset=p;}catch(e){}})();`

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
      className={`${bricolageSans.variable} ${geistMono.variable} font-sans antialiased dark`}
      suppressHydrationWarning
    >
      <body className={bricolageSans.className}>
        <PublicConfigScript />
        <script dangerouslySetInnerHTML={{ __html: PRESET_NO_FLASH }} />
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
