import { createHmac } from "node:crypto";
import { describe, expect, it } from "vitest";
import { verifySignature } from "../src/webhook-signature.js";

const SECRET = "whsec_4f3e2d1c0b9a";
const BODY = JSON.stringify({ event_id: "01890a5d", type: "ledger.transaction.posted" });

function sign(secret: string, t: number, body: string): string {
  return createHmac("sha256", secret).update(`${t}.${body}`).digest("hex");
}

describe("verifySignature", () => {
  const t = 1_753_000_000;

  it("accepts a valid signature", async () => {
    const header = `t=${t},v1=${sign(SECRET, t, BODY)}`;
    await expect(verifySignature(BODY, header, SECRET, { now: t })).resolves.toBe(true);
  });

  it("accepts any matching candidate during secret rotation", async () => {
    const header = `t=${t},v1=${sign("whsec_old", t, BODY)},v1=${sign(SECRET, t, BODY)}`;
    await expect(verifySignature(BODY, header, SECRET, { now: t })).resolves.toBe(true);
  });

  it("rejects a tampered body", async () => {
    const header = `t=${t},v1=${sign(SECRET, t, BODY)}`;
    await expect(verifySignature(BODY + "x", header, SECRET, { now: t })).resolves.toBe(false);
  });

  it("rejects a tampered timestamp", async () => {
    const header = `t=${t + 1},v1=${sign(SECRET, t, BODY)}`;
    await expect(verifySignature(BODY, header, SECRET, { now: t })).resolves.toBe(false);
  });

  it("rejects the wrong secret", async () => {
    const header = `t=${t},v1=${sign(SECRET, t, BODY)}`;
    await expect(verifySignature(BODY, header, "whsec_other", { now: t })).resolves.toBe(false);
  });

  it("rejects stale timestamps outside the tolerance window", async () => {
    const header = `t=${t},v1=${sign(SECRET, t, BODY)}`;
    await expect(verifySignature(BODY, header, SECRET, { now: t + 301 })).resolves.toBe(false);
    await expect(
      verifySignature(BODY, header, SECRET, { now: t + 301, toleranceSeconds: 0 }),
    ).resolves.toBe(true);
  });

  it("rejects malformed or missing headers", async () => {
    await expect(verifySignature(BODY, undefined, SECRET)).resolves.toBe(false);
    await expect(verifySignature(BODY, "", SECRET)).resolves.toBe(false);
    await expect(verifySignature(BODY, "garbage", SECRET)).resolves.toBe(false);
    await expect(verifySignature(BODY, `v1=${sign(SECRET, t, BODY)}`, SECRET)).resolves.toBe(false);
    await expect(verifySignature(BODY, `t=${t}`, SECRET, { now: t })).resolves.toBe(false);
  });
});
