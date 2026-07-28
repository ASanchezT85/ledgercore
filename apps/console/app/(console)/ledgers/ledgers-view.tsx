"use client";

import { BookOpenText, CornerDownRight, FolderTree } from "lucide-react";
import { DemoBadge } from "@/components/demo-badge";
import {
  ApiDownBanner,
  EmptyState,
  ErrorBanner,
  LoadingCard,
  useLiveData,
} from "@/components/live";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { formatMoneyWithAsset } from "@/lib/format";
import { loadLedgersTree, type LedgerWithAccounts } from "@/lib/live";
import { mockLedgers } from "@/lib/mock-data";

const CREATE_LEDGER_CURL = `curl -s -X POST $API/v1/ledgers \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{ "name": "main", "description": "Primary operating ledger" }'`;

interface DisplayBalance {
  asset: string;
  exponent: number;
  units: bigint;
}

interface TreeNode {
  name: string;
  fullPath: string;
  balances: DisplayBalance[] | null;
  status?: string;
  children: TreeNode[];
}

/**
 * Builds a nested tree out of flat account names. Live accounts follow the
 * colon-namespaced convention (customer:cust_42:wallet); the demo dataset
 * uses slash-separated paths. Both are treated as segment separators.
 */
function buildTree(
  accounts: Array<{ name: string; balances: DisplayBalance[]; status?: string }>,
): TreeNode[] {
  const roots: TreeNode[] = [];
  for (const account of accounts) {
    const segments = account.name.split(/[:/]/).filter(Boolean);
    let level = roots;
    let prefix = "";
    segments.forEach((segment, index) => {
      prefix = prefix ? `${prefix}/${segment}` : segment;
      let node = level.find((candidate) => candidate.name === segment);
      if (!node) {
        node = { name: segment, fullPath: prefix, balances: null, children: [] };
        level.push(node);
      }
      if (index === segments.length - 1) {
        node.balances = account.balances;
        node.status = account.status;
      }
      level = node.children;
    });
  }
  return roots;
}

function TreeBranch({ node, depth }: { node: TreeNode; depth: number }) {
  return (
    <li>
      <div
        className="flex items-center justify-between gap-4 border-b border-edge py-2.5 last:border-b-0"
        style={{ paddingLeft: `${depth * 1.5}rem` }}
      >
        <div className="flex min-w-0 items-center gap-2">
          {depth > 0 && (
            <CornerDownRight
              size={13}
              className="shrink-0 text-ink-faint"
              aria-hidden="true"
            />
          )}
          {node.balances ? (
            <span className="truncate font-mono text-[13px] text-ink">
              {node.name}
            </span>
          ) : (
            <span className="truncate text-[11px] font-semibold tracking-wider text-ink-faint uppercase">
              {node.name}
            </span>
          )}
          {node.status && node.status !== "active" && (
            <Badge tone="amber">{node.status}</Badge>
          )}
        </div>
        {node.balances && (
          <div className="flex shrink-0 flex-col items-end gap-0.5">
            {node.balances.length === 0 && (
              <span className="text-xs text-ink-faint">sin movimientos</span>
            )}
            {node.balances.map((balance) => (
              <span
                key={balance.asset}
                className="num text-[13px] font-medium text-ink"
              >
                {formatMoneyWithAsset(balance.units, balance.asset, balance.exponent)}
              </span>
            ))}
          </div>
        )}
      </div>
      {node.children.length > 0 && (
        <ul>
          {node.children.map((child) => (
            <TreeBranch key={child.fullPath} node={child} depth={depth + 1} />
          ))}
        </ul>
      )}
    </li>
  );
}

function LedgerCard({
  name,
  description,
  environment,
  accounts,
}: {
  name: string;
  description: string;
  environment?: string;
  accounts: Array<{ name: string; balances: DisplayBalance[]; status?: string }>;
}) {
  return (
    <Card
      title={name}
      subtitle={description}
      actions={
        <span className="flex items-center gap-2">
          {environment && (
            <Badge tone={environment === "live" ? "emerald" : "violet"}>
              {environment}
            </Badge>
          )}
          <Badge tone="slate" className="gap-1">
            <FolderTree size={11} aria-hidden="true" />
            {accounts.length} cuentas
          </Badge>
        </span>
      }
    >
      {accounts.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-8 text-center">
          <BookOpenText size={22} className="text-ink-faint" aria-hidden="true" />
          <p className="text-sm text-ink-muted">
            Este ledger aún no tiene cuentas. Créalas con{" "}
            <code className="font-mono text-xs text-accent">POST /v1/accounts</code>{" "}
            (guía en{" "}
            <a href="/docs/quickstart" className="text-accent hover:underline">
              /docs/quickstart
            </a>
            ).
          </p>
        </div>
      ) : (
        <ul>
          {buildTree(accounts).map((node) => (
            <TreeBranch key={node.fullPath} node={node} depth={0} />
          ))}
        </ul>
      )}
    </Card>
  );
}

function LiveLedgers({ data }: { data: LedgerWithAccounts[] }) {
  if (data.length === 0) {
    return (
      <EmptyState
        title="Aún no hay ledgers en este tenant"
        body="Un ledger es un libro contable aislado con su propio árbol de cuentas. Crea el primero con este curl y recarga."
        curl={CREATE_LEDGER_CURL}
        curlLabel="POST /v1/ledgers"
      />
    );
  }
  return (
    <>
      {data.map(({ ledger, accounts }) => (
        <LedgerCard
          key={ledger.id}
          name={ledger.name}
          description={ledger.description ?? ""}
          environment={ledger.environment}
          accounts={accounts.map(({ account, balances }) => ({
            name: account.name,
            status: account.status,
            balances: balances.map((b) => ({
              asset: b.asset,
              exponent: b.exponent,
              units: BigInt(b.posted),
            })),
          }))}
        />
      ))}
    </>
  );
}

function DemoLedgers() {
  return (
    <>
      {mockLedgers.map((ledger) => (
        <LedgerCard
          key={ledger.id}
          name={ledger.name}
          description={ledger.description}
          accounts={ledger.accounts.map((account) => ({
            name: account.path,
            balances: account.balances.map((b) => ({
              asset: b.asset,
              exponent: b.exponent,
              units: BigInt(Math.trunc(b.units)),
            })),
          }))}
        />
      ))}
    </>
  );
}

export function LedgersView() {
  const state = useLiveData(loadLedgersTree);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink">
            Ledgers
          </h1>
          <p className="mt-0.5 text-sm text-ink-faint">
            Libros contables y árbol de cuentas con balances por asset
          </p>
        </div>
        <DemoBadge demo={state.status === "demo"} />
      </div>

      {state.status === "loading" && <LoadingCard orb="solving" />}
      {state.status === "demo" && <DemoLedgers />}
      {state.status === "down" && <ApiDownBanner onRetry={state.reload} />}
      {state.status === "error" && (
        <ErrorBanner message={state.message} onRetry={state.reload} />
      )}
      {state.status === "live" && <LiveLedgers data={state.data} />}
    </div>
  );
}
