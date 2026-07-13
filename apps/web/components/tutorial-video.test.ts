import { describe, expect, it } from "vitest"
import { youtubeEmbedURL } from "@/components/tutorial-video"

describe("youtubeEmbedURL", () => {
  it("normalizes common YouTube links to a privacy-enhanced embed", () => {
    expect(youtubeEmbedURL("https://youtu.be/abc123XYZ_0")).toBe(
      "https://www.youtube-nocookie.com/embed/abc123XYZ_0?rel=0"
    )
    expect(
      youtubeEmbedURL("https://www.youtube.com/watch?v=abc123XYZ_0")
    ).toBe("https://www.youtube-nocookie.com/embed/abc123XYZ_0?rel=0")
  })

  it("rejects non-YouTube and malformed links", () => {
    expect(youtubeEmbedURL("https://example.com/video")).toBeNull()
    expect(youtubeEmbedURL("not a url")).toBeNull()
  })
})
