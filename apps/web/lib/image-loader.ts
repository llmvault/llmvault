import { clientConfig } from "@/lib/config/public-config"

// Passthrough image loader (no image optimization): rewrites relative asset paths
// to the runtime-configured assets host so a single prebuilt image works against
// any domain. This replaces the build-time `images.remotePatterns` allowlist,
// which was evaluated at build time and hardcoded a single assets host.
export default function hivyImageLoader({ src }: { src: string; width: number; quality?: number }): string {
  // Absolute URLs are returned unchanged.
  if (src.startsWith("http://") || src.startsWith("https://")) {
    return src
  }
  // Relative paths are prefixed with the runtime assets base URL.
  return `${clientConfig().assetsUrl.replace(/\/$/, "")}/${src.replace(/^\//, "")}`
}
