import { NextRequest, NextResponse } from "next/server"

const SESSION_COOKIE = "__session"
const AUTH_ROUTES = new Set([
  "/auth/signin",
  "/auth/login",
  "/auth/signup",
  "/hero/auth/login",
  "/hero/auth/signup",
])

export function proxy(req: NextRequest) {
  const hasSession = req.cookies.has(SESSION_COOKIE)
  const { pathname } = req.nextUrl

  // Protected routes: require a session.
  if ((pathname === "/w" || pathname.startsWith("/w/")) && !hasSession) {
    return NextResponse.redirect(new URL("/auth/signin", req.url))
  }

  if (
    (pathname === "/hero/w" || pathname.startsWith("/hero/w/")) &&
    !hasSession
  ) {
    return NextResponse.redirect(new URL("/hero/auth/login", req.url))
  }

  if (AUTH_ROUTES.has(pathname) && hasSession) {
    const workspacePath = pathname.startsWith("/hero/") ? "/hero/w" : "/w"
    return NextResponse.redirect(new URL(workspacePath, req.url))
  }

  return NextResponse.next()
}

export const config = {
  matcher: [
    "/w/:path*",
    "/hero/w/:path*",
    "/auth/signin",
    "/auth/login",
    "/auth/signup",
    "/hero/auth/login",
    "/hero/auth/signup",
  ],
}
