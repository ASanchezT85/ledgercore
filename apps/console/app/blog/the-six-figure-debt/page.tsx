import type { Metadata } from "next";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { BrandLockup } from "@/components/logo";

const TITLE = "The six-figure debt that existed in no system";
const DESCRIPTION =
  "A payment provider said we owed them six figures. Our systems said we owed them nothing. Five scars from operating money movement on mutable tables — and the ledger principles each one burned in.";

export const metadata: Metadata = {
  title: `${TITLE} · LedgerCore`,
  description: DESCRIPTION,
  authors: [{ name: "Alexander Sanchez" }],
  openGraph: {
    type: "article",
    title: TITLE,
    description: DESCRIPTION,
    siteName: "LedgerCore",
    publishedTime: "2026-07-28T00:00:00.000Z",
    authors: ["Alexander Sanchez"],
  },
  twitter: {
    card: "summary_large_image",
    title: TITLE,
    description: DESCRIPTION,
  },
};

const SQL_BALANCE_CHECK = `-- Reject any transaction whose postings don't balance per asset
SELECT tx_id, asset, SUM(amount) AS delta
FROM postings
WHERE tx_id = $1
GROUP BY tx_id, asset
HAVING SUM(amount) <> 0;
-- one row back => the write is refused`;

const SQL_APPEND_ONLY = `CREATE FUNCTION ledger_no_rewrite() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'ledger is append-only; post a reversal instead';
END $$ LANGUAGE plpgsql;

CREATE TRIGGER postings_immutable
  BEFORE UPDATE OR DELETE ON postings
  FOR EACH ROW EXECUTE FUNCTION ledger_no_rewrite();`;

function P({ children }: { children: React.ReactNode }) {
  return (
    <p className="mt-6 text-[1.0625rem] leading-[1.85] text-ink-muted">
      {children}
    </p>
  );
}

function H2({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mt-12 text-2xl font-semibold tracking-tight text-ink">
      {children}
    </h2>
  );
}

function Strong({ children }: { children: React.ReactNode }) {
  return <strong className="font-semibold text-ink">{children}</strong>;
}

function Code({ children }: { children: React.ReactNode }) {
  return (
    <code className="rounded bg-surface-raised px-1.5 py-0.5 font-mono text-[0.85em] text-ink">
      {children}
    </code>
  );
}

function SqlBlock({ code }: { code: string }) {
  return (
    <div className="mt-6 overflow-hidden rounded-(--radius-card) border border-edge bg-surface">
      <div className="flex items-center justify-between border-b border-edge px-4 py-2.5">
        <span className="font-mono text-[11px] text-ink-faint">SQL</span>
        <span className="text-[10px] tracking-widest text-ink-faint uppercase">
          postgres
        </span>
      </div>
      <pre className="overflow-x-auto p-4 font-mono text-[13px] leading-relaxed text-ink-muted">
        <code>{code}</code>
      </pre>
    </div>
  );
}

export default function SixFigureDebtPost() {
  return (
    <div className="min-h-screen">
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <Link href="/" aria-label="LedgerCore home">
          <BrandLockup size={30} />
        </Link>
        <nav className="flex items-center gap-2" aria-label="Main">
          <Link
            href="/blog"
            className="inline-flex items-center gap-1.5 rounded-(--radius-control) px-4 py-2 text-sm text-ink-muted transition-colors hover:text-ink"
          >
            <ArrowLeft size={14} aria-hidden="true" />
            Blog
          </Link>
          <Link
            href="/signup"
            className="rounded-(--radius-control) border border-edge-strong px-4 py-2 text-sm font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent"
          >
            Try the sandbox
          </Link>
        </nav>
      </header>

      <main className="mx-auto w-full max-w-prose px-6 pt-10 pb-24">
        <article>
          <header>
            <time
              dateTime="2026-07-28"
              className="text-xs tracking-wide text-ink-faint"
            >
              July 28, 2026
            </time>
            <h1 className="mt-3 text-3xl font-semibold tracking-tight text-ink sm:text-4xl sm:leading-[1.2]">
              {TITLE}
            </h1>
            <p className="mt-4 text-sm text-ink-faint">
              Alexander Sanchez — builder of LedgerCore
            </p>
          </header>

          <P>
            I found it on a Tuesday, cross-referencing a payment
            provider&apos;s settlement statement against our own books. The
            provider said we owed them six figures. Our systems said we owed
            them nothing. Not &ldquo;a smaller number&rdquo; — nothing. The
            debt did not exist anywhere inside the company.
          </P>
          <P>
            I spent years operating money-movement infrastructure for a
            remittance platform. Real volume, real regulators, real payout
            partners in multiple countries. This post is about the scars that
            operation left on me, and the design principles I refuse to
            compromise on now. If you&apos;re building anything that touches
            money on top of ordinary application tables, some of this will
            feel uncomfortably familiar.
          </P>

          <H2>Why operational tables + spreadsheets fail structurally</H2>
          <P>
            Most fintech-adjacent systems start the same way: an{" "}
            <Code>orders</Code>-style table, a <Code>balance</Code> column
            somewhere, and a growing constellation of spreadsheets doing
            reconciliation on the side. It works right up until it
            doesn&apos;t, and the failure is structural, not incidental:
          </P>
          <ul className="mt-6 list-disc space-y-3 pl-6 text-[1.0625rem] leading-[1.85] text-ink-muted marker:text-accent">
            <li>
              Operational tables are mutable. A row that says
              &ldquo;completed&rdquo; today can say something else tomorrow,
              and nothing records the transition.
            </li>
            <li>
              Balances stored as columns drift from the transactions that
              supposedly produced them, because updates to the two are not
              atomic.
            </li>
            <li>
              Spreadsheets are the escape hatch, and spreadsheets are hostile
              to correctness: studies have found errors in roughly 94% of
              spreadsheets in use, and around 52% of companies report material
              reconciliation exceptions in their close process.
            </li>
          </ul>
          <P>
            None of that is because people are careless. It&apos;s because the
            data model has no concept of financial truth. It has a concept of{" "}
            <em className="text-ink">current state</em>, which is a different,
            weaker thing.
          </P>
          <P>
            Here are the five scars that taught me that, each with the lesson
            it burned in.
          </P>

          <H2>Scar 1: The debt that didn&apos;t exist</H2>
          <P>
            When we finally built a proper ledger, we seeded it with an
            opening entry derived from current balances. The seeding logic had
            one quiet assumption: negative balances were noise, so it
            discarded them.
          </P>
          <P>
            One of those &ldquo;noise&rdquo; values was our position with a
            payout provider. We held a{" "}
            <em className="text-ink">negative</em> custody balance with them —
            they had paid out on our behalf beyond what we had funded. That is
            a liability. A real, six-figure debt. The opening entry threw it
            away, so the ledger was born already lying, and every report
            downstream inherited the lie.
          </P>
          <P>
            <Strong>Lesson:</Strong> a negative custody balance is a
            liability, not an anomaly. Any ledger bootstrap that filters by
            sign is silently choosing which reality to keep. Double-entry
            makes this class of bug loud: you cannot discard one side of a
            position without the books failing to balance.
          </P>

          <H2>Scar 2: The wallet that was 97% frozen</H2>
          <P>
            A business customer&apos;s wallet showed ~97% of its balance
            locked in reserves — with zero open transactions. Weeks of holds,
            no releases.
          </P>
          <P>
            Root cause: the reserve-release logic detected &ldquo;this order
            used the wallet&rdquo; by checking a collateral field on a
            payment-detail record. A newer transaction flow simply didn&apos;t
            create that record. Orders completed fine; the release check found
            nothing to release; reserves accumulated forever.
          </P>
          <P>
            <Strong>Lesson:</Strong> if a money-affecting decision depends on
            the <em className="text-ink">incidental shape</em> of operational
            data, every new flow is a chance to break it silently. Holds and
            releases must be first-class ledger entries — a hold is a posting,
            a release is a posting, and &ldquo;how much is reserved&rdquo; is
            a balance, not an inference over side tables.
          </P>

          <H2>Scar 3: Balances computed by summing everything, every time</H2>
          <P>
            Balance checks worked by loading all of a wallet&apos;s movement
            rows and summing them in memory. On every request. It was slow, it
            got slower linearly with history, and worse: two concurrent
            requests could both read the same &ldquo;available&rdquo; figure
            and both spend it.
          </P>
          <P>
            <Strong>Lesson:</Strong> materialize balances in the same database
            transaction as the posting that changes them. The posting rows
            remain the source of truth; the materialized balance is a cache
            that is <em className="text-ink">never allowed to drift</em>,
            because it moves atomically with the entries. You get O(1) reads
            and a natural place to enforce overdraft rules.
          </P>

          <H2>Scar 4: &ldquo;1.500&rdquo; became 1500</H2>
          <P>
            We had a helper that normalized user-entered decimal amounts.
            Given <Code>&quot;1.500&quot;</Code>, it decided the dot was a
            thousands separator and returned 1500. A ×1000 error in a money
            path.
          </P>
          <P>
            The best part: there was a unit test asserting exactly that
            behavior. The bug had been enshrined as a specification. Whoever
            wrote the test looked at the wrong output and wrote it down as
            correct.
          </P>
          <P>
            <Strong>Lesson:</Strong> floats and locale-sniffing string parsers
            have no place near money. Amounts should be integers in minor
            units, tagged with an asset code and an exponent (
            <Code>amount: 150000, asset: &quot;USD&quot;, exponent: 2</Code>).
            Parsing happens once, at the edge, against an explicit format —
            never by guessing what a dot means.
          </P>

          <H2>Scar 5: Weeks of forensic SQL</H2>
          <P>
            Every incident above ended the same way: me, a read replica, and
            hand-written SQL, reconstructing what{" "}
            <em className="text-ink">must have happened</em> from mutable rows
            that only stored what things looked like{" "}
            <em className="text-ink">now</em>. Some questions were simply
            unanswerable — the intermediate states were gone.
          </P>
          <P>
            <Strong>Lesson:</Strong> if your data model can&apos;t replay
            history, every incident becomes archaeology. Append-only
            isn&apos;t a purity preference; it&apos;s the difference between
            &ldquo;query the timeline&rdquo; and &ldquo;interview the
            survivors.&rdquo;
          </P>

          <H2>The principles I built into the ledger afterwards</H2>
          <P>
            These aren&apos;t aspirations. Each one is a scar with the
            polarity reversed.
          </P>
          <P>
            <Strong>1. Double-entry, enforced, not encouraged.</Strong> Every
            transaction is a set of postings that sums to zero per asset. Not
            by convention — by check, at write time:
          </P>
          <SqlBlock code={SQL_BALANCE_CHECK} />
          <P>
            If this had existed on day one, Scar 1 is impossible: dropping the
            liability side of the opening entry fails to commit.
          </P>
          <P>
            <Strong>
              2. Append-only, with corrections as compensating entries.
            </Strong>{" "}
            No UPDATE, no DELETE on posting tables — enforced in the database
            itself, so even a well-intentioned migration script can&apos;t
            rewrite history:
          </P>
          <SqlBlock code={SQL_APPEND_ONLY} />
          <P>
            Made a mistake? Post the reversal and the corrected entry. The
            mistake stays visible. That visibility is the feature.
          </P>
          <P>
            <Strong>3. Money as integers: asset + exponent.</Strong> No
            floats, no strings, no locale guessing. <Code>1500</Code> USD is{" "}
            <Code>
              {"{amount: 150000, asset: \"USD\", exponent: 2}"}
            </Code>{" "}
            everywhere below the presentation layer. Scar 4 becomes a
            rendering question, not a solvency question.
          </P>
          <P>
            <Strong>4. Idempotency end to end.</Strong> Every external command
            carries an idempotency key; replays return the original result
            instead of posting twice. Retries, webhook redeliveries, and
            impatient clients stop being money events.
          </P>
          <P>
            <Strong>5. Balances materialized in the same transaction.</Strong>{" "}
            Posting and balance update commit together or not at all. Reads
            are O(1), overdraft checks are race-free, and the balance can be
            re-derived from postings at any time to prove it.
          </P>
          <P>
            <Strong>
              6. Continuous reconciliation against the outside world.
            </Strong>{" "}
            The provider&apos;s statement, the bank feed, the processor report
            — ingested and diffed against the ledger continuously, not
            quarterly. Scar 1 was discovered by accident; it should have been
            an alert within a day.
          </P>
          <P>
            None of these ideas are new. Accountants have had double-entry for
            five centuries. What&apos;s new is how consistently software teams
            rediscover, at production scale and real cost, why every one of
            these rules exists.
          </P>
          <P>
            I got tired of rebuilding this from scars, so I&apos;m building it
            as a product: LedgerCore, a ledger-as-a-service around exactly
            these principles.{" "}
            <a
              href="https://ledgercore.sanchezavila.com"
              className="font-medium text-accent underline decoration-accent/40 underline-offset-4 transition-colors hover:decoration-accent"
            >
              There&apos;s a sandbox
            </a>{" "}
            if you want to poke at it. Either way — if you&apos;re storing
            balances in a mutable column right now, go check how your opening
            balances were seeded. I&apos;ll wait.
          </P>
        </article>
      </main>

      <footer className="border-t border-edge">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-4 px-6 py-8">
          <BrandLockup size={22} />
          <p className="text-[11px] tracking-wide text-ink-faint">
            LedgerCore — financial infrastructure
          </p>
        </div>
      </footer>
    </div>
  );
}
