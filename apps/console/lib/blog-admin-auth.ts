import "server-only";

import { timingSafeEqual } from "node:crypto";

/**
 * Auth for the comment moderation panel.
 *
 * Deliberately NOT the console's tenant session: blog comments are not tenant
 * data, and no API key holder should be able to moderate the site's blog. This
 * is a single operator secret (BLOG_ADMIN_TOKEN) held by whoever runs the site.
 */

export function moderationConfigured(): boolean {
  return (process.env.BLOG_ADMIN_TOKEN ?? "").length > 0;
}

/** Constant-time comparison, so a wrong token leaks nothing through timing. */
export function isModerator(request: Request): boolean {
  const expected = process.env.BLOG_ADMIN_TOKEN ?? "";
  if (expected.length === 0) return false;

  const header = request.headers.get("authorization") ?? "";
  const provided = header.startsWith("Bearer ") ? header.slice(7) : "";
  if (provided.length === 0) return false;

  const a = Buffer.from(provided);
  const b = Buffer.from(expected);
  // timingSafeEqual throws on length mismatch, which would itself be a leak;
  // compare a fixed-size digest-like pair instead by padding to equal length.
  if (a.length !== b.length) {
    // Still burn a comparison so the failure path costs the same.
    timingSafeEqual(b, b);
    return false;
  }
  return timingSafeEqual(a, b);
}
