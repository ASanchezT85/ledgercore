import type { Metadata } from "next";
import { BookOpenText, CornerDownRight, FolderTree } from "lucide-react";
import { DemoBadge } from "@/components/demo-badge";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { getLedgers } from "@/lib/api";
import { formatMoneyWithAsset } from "@/lib/format";
import type { LedgerAccount } from "@/lib/types";

export const metadata: Metadata = {
  title: "Ledgers",
};

export const dynamic = "force-dynamic";

interface TreeNode {
  name: string;
  fullPath: string;
  account?: LedgerAccount;
  children: TreeNode[];
}

/** Builds a nested tree out of flat slash-separated account paths. */
function buildTree(accounts: LedgerAccount[]): TreeNode[] {
  const roots: TreeNode[] = [];

  for (const account of accounts) {
    const segments = account.path.split("/");
    let level = roots;
    let prefix = "";

    segments.forEach((segment, index) => {
      prefix = prefix ? `${prefix}/${segment}` : segment;
      let node = level.find((candidate) => candidate.name === segment);
      if (!node) {
        node = { name: segment, fullPath: prefix, children: [] };
        level.push(node);
      }
      if (index === segments.length - 1) {
        node.account = account;
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
          {node.account ? (
            <span className="truncate font-mono text-[13px] text-ink">
              {node.name}
            </span>
          ) : (
            <span className="truncate text-[11px] font-semibold tracking-wider text-ink-faint uppercase">
              {node.name}
            </span>
          )}
        </div>
        {node.account && (
          <div className="flex shrink-0 flex-col items-end gap-0.5">
            {node.account.balances.map((balance) => (
              <span
                key={balance.asset}
                className="num text-[13px] font-medium text-ink"
              >
                {formatMoneyWithAsset(
                  balance.units,
                  balance.asset,
                  balance.exponent,
                )}
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

export default async function LedgersPage() {
  const { data: ledgers, demo } = await getLedgers();

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
        <DemoBadge demo={demo} />
      </div>

      {ledgers.map((ledger) => (
        <Card
          key={ledger.id}
          title={ledger.name}
          subtitle={ledger.description}
          actions={
            <Badge tone="slate" className="gap-1">
              <FolderTree size={11} aria-hidden="true" />
              {ledger.accounts.length} cuentas
            </Badge>
          }
        >
          <ul>
            {buildTree(ledger.accounts).map((node) => (
              <TreeBranch key={node.fullPath} node={node} depth={0} />
            ))}
          </ul>
        </Card>
      ))}

      {ledgers.length === 0 && (
        <Card>
          <div className="flex flex-col items-center gap-2 py-10 text-center">
            <BookOpenText size={24} className="text-ink-faint" aria-hidden="true" />
            <p className="text-sm text-ink-muted">
              Aún no hay ledgers en este tenant.
            </p>
          </div>
        </Card>
      )}
    </div>
  );
}
