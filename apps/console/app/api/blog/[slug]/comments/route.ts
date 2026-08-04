import { NextResponse } from "next/server";
import {
  CommentRejected,
  clientHash,
  clientIp,
  createComment,
  listComments,
} from "@/lib/blog-db";
import { isKnownPost } from "@/lib/blog-posts";

export const dynamic = "force-dynamic";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ slug: string }> },
) {
  const { slug } = await params;
  if (!isKnownPost(slug)) {
    return NextResponse.json({ error: "unknown_post" }, { status: 404 });
  }
  try {
    return NextResponse.json({ comments: await listComments(slug) });
  } catch {
    // A blog outage must never look like a broken post: return an empty thread.
    return NextResponse.json({ comments: [], degraded: true });
  }
}

export async function POST(
  request: Request,
  { params }: { params: Promise<{ slug: string }> },
) {
  const { slug } = await params;
  if (!isKnownPost(slug)) {
    return NextResponse.json({ error: "unknown_post" }, { status: 404 });
  }

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid_request" }, { status: 400 });
  }

  const payload = body as {
    author_name?: unknown;
    body?: unknown;
    parent_id?: unknown;
    website?: unknown; // honeypot: real humans never see this field
  };

  // Honeypot filled => a bot. Answer 201 so it does not learn it was caught.
  if (typeof payload.website === "string" && payload.website.length > 0) {
    return NextResponse.json({ comment: null, accepted: false }, { status: 201 });
  }

  const authorName = typeof payload.author_name === "string" ? payload.author_name.trim() : "";
  const text = typeof payload.body === "string" ? payload.body.trim() : "";
  const parentId = typeof payload.parent_id === "string" && payload.parent_id ? payload.parent_id : null;

  if (authorName.length < 2 || authorName.length > 60 || text.length < 2 || text.length > 4000) {
    return NextResponse.json({ error: "invalid" }, { status: 422 });
  }

  const authorHash = clientHash([clientIp(request.headers)]);

  try {
    const comment = await createComment({
      slug,
      parentId,
      authorName,
      body: text,
      authorHash,
    });
    return NextResponse.json({ comment, accepted: true }, { status: 201 });
  } catch (err) {
    if (err instanceof CommentRejected) {
      const status =
        err.reason === "rate_limited" ? 429 : err.reason === "unavailable" ? 503 : 422;
      return NextResponse.json({ error: err.reason }, { status });
    }
    return NextResponse.json({ error: "unavailable" }, { status: 503 });
  }
}
