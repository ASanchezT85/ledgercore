import { NextResponse } from "next/server";
import { isModerator, moderationConfigured } from "@/lib/blog-admin-auth";
import {
  deleteComment,
  listAllComments,
  setCommentStatus,
} from "@/lib/blog-db";

export const dynamic = "force-dynamic";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function unauthorized() {
  return NextResponse.json({ error: "unauthorized" }, { status: 401 });
}

/** Full list, hidden included. */
export async function GET(request: Request) {
  if (!moderationConfigured()) {
    return NextResponse.json({ error: "not_configured" }, { status: 503 });
  }
  if (!isModerator(request)) return unauthorized();
  try {
    return NextResponse.json({ comments: await listAllComments() });
  } catch {
    return NextResponse.json({ error: "unavailable" }, { status: 503 });
  }
}

/** Hide or restore. Body: { id, status: "visible" | "hidden" }. */
export async function PATCH(request: Request) {
  if (!isModerator(request)) return unauthorized();

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid_request" }, { status: 400 });
  }
  const { id, status } = body as { id?: unknown; status?: unknown };

  if (typeof id !== "string" || !UUID.test(id)) {
    return NextResponse.json({ error: "invalid_id" }, { status: 422 });
  }
  if (status !== "visible" && status !== "hidden") {
    return NextResponse.json({ error: "invalid_status" }, { status: 422 });
  }

  try {
    const ok = await setCommentStatus(id, status);
    if (!ok) return NextResponse.json({ error: "not_found" }, { status: 404 });
    return NextResponse.json({ id, status });
  } catch {
    return NextResponse.json({ error: "unavailable" }, { status: 503 });
  }
}

/** Permanent removal. Body: { id }. Replies cascade. */
export async function DELETE(request: Request) {
  if (!isModerator(request)) return unauthorized();

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid_request" }, { status: 400 });
  }
  const { id } = body as { id?: unknown };

  if (typeof id !== "string" || !UUID.test(id)) {
    return NextResponse.json({ error: "invalid_id" }, { status: 422 });
  }

  try {
    const ok = await deleteComment(id);
    if (!ok) return NextResponse.json({ error: "not_found" }, { status: 404 });
    return NextResponse.json({ id, deleted: true });
  } catch {
    return NextResponse.json({ error: "unavailable" }, { status: 503 });
  }
}
