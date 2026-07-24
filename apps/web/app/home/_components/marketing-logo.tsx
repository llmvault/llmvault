import { cn } from "@/lib/utils"

export function MarketingLogo({ className }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="15 46.8 99 77"
      fill="currentColor"
      aria-hidden="true"
      focusable="false"
      className={cn("block shrink-0", className)}
    >
      <path d="m26.9 63-6.8 2v33.7-0.3h6.9v-13.1c0-3.3 1.5-5.7 4.7-5.7 3 0 4.2 2.3 4.3 4.6v14.2h6.9v-14.6c0-5.9-3.4-10.3-8.8-10.3-2.2-0.1-4.6 0.6-7 3v-13.7l-0.2 0.2z" />
      <path d="m47.6 63.3h7.2v6.6h-7.2v-6.6zm0 10.6h7.1v24.5h-7.1v-24.5z" />
      <path d="m57.1 73.9 9.8 24.5h6.9l9.4-23.4 6.3 15.1 3 6.8-4.2 10.6h7.2l13.6-33.6h-7.1l-5.8 15-5.2-15h-14.8l-5.8 16.4-5.8-16.4z" />
    </svg>
  )
}
