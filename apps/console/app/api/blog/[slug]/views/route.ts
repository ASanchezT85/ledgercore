import { NextResponse } from "next/server";
import { clientHash, clientIp, recordView } from "@/lib/blog-db";
import { isKnownPost } from "@/lib/blog-posts";

export const dynamic = "force-dynamic";

/**
 * Called once by the post page after it renders. Counting on the client (rather
 * than during SSR) keeps prefetches, crawlers and health checks out of the
 * number — we would rather under-count than publish an inflated one.
 */
export async function POST(
  request: Request,
  { params }: { params: Promise<{ slug: string }> },
) {
  const { slug } = await params;
  if (!isKnownPost(slug)) {
    return NextResponse.json({ error: "unknown_post" }, { status: 404 });
  }
  const visitorHash = clientHash([
    clientIp(request.headers),
    request.headers.get("user-agent") ?? "",
  ]);
  try {
    return NextResponse.json({ views: await recordView(slug, visitorHash) });
  } catch {
    return NextResponse.json({ views: null, degraded: true });
  }
}
