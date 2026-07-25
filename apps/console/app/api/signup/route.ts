import { NextResponse } from "next/server";

// Server-side proxy for the public sandbox signup: the browser posts here
// (same origin) and this handler forwards to the identity service through
// the gateway (NEXT_PUBLIC_API_URL is resolvable server-side inside the
// compose network). The API key travels back exactly once and is never
// stored by the console.
export async function POST(request: Request) {
  const base = process.env.NEXT_PUBLIC_API_URL;
  if (!base) {
    return NextResponse.json(
      { error: "api_unconfigured", message: "NEXT_PUBLIC_API_URL is not set" },
      { status: 503 },
    );
  }
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "invalid_request", message: "body must be JSON" },
      { status: 400 },
    );
  }
  try {
    const upstream = await fetch(`${base}/v1/sandbox/signups`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const payload = await upstream.json().catch(() => ({}));
    return NextResponse.json(payload, { status: upstream.status });
  } catch {
    return NextResponse.json(
      { error: "gateway_unreachable", message: "could not reach the LedgerCore gateway" },
      { status: 502 },
    );
  }
}
