/**
 * Realistic, internally-consistent demo data. Served whenever the LedgerCore
 * API is unreachable or NEXT_PUBLIC_API_URL is unset, so the console can be
 * demoed with zero backing services. All amounts are integer minor units.
 */

import type {
  DashboardData,
  DeveloperData,
  LedgerSummary,
  ReconciliationData,
  Transaction,
} from "./types";

const USD = { asset: "USD", exponent: 2 } as const;
const EUR = { asset: "EUR", exponent: 2 } as const;
const MXN = { asset: "MXN", exponent: 2 } as const;

export const mockTransactions: Transaction[] = [
  {
    id: "txn_01J8ZC9QK4W2",
    ledger: "main",
    description: "Customer deposit — ACH inbound",
    status: "posted",
    amountUnits: 10000,
    ...USD,
    createdAt: "2026-07-24T14:32:08Z",
    idempotencyKey: "dep_9f2c41d7",
    postings: [
      {
        account: "assets/custody/bank/bofa",
        direction: "debit",
        amountUnits: 10000,
        ...USD,
      },
      {
        account: "liabilities/customers/cus_8c31/wallet",
        direction: "credit",
        amountUnits: 9700,
        ...USD,
      },
      {
        account: "revenue/fees/deposit",
        direction: "credit",
        amountUnits: 300,
        ...USD,
      },
    ],
  },
  {
    id: "txn_01J8ZC7P2N8H",
    ledger: "main",
    description: "Payout to provider — cross-border transfer",
    status: "posted",
    amountUnits: 2547500,
    ...USD,
    createdAt: "2026-07-24T14:18:41Z",
    idempotencyKey: "pay_4b81aa02",
    postings: [
      {
        account: "liabilities/customers/cus_2f77/wallet",
        direction: "debit",
        amountUnits: 2547500,
        ...USD,
      },
      {
        account: "assets/custody/provider/acmepay",
        direction: "credit",
        amountUnits: 2521300,
        ...USD,
      },
      {
        account: "revenue/fees/payout",
        direction: "credit",
        amountUnits: 26200,
        ...USD,
      },
    ],
  },
  {
    id: "txn_01J8ZC5M9XQ3",
    ledger: "main",
    description: "Card top-up via PSP",
    status: "pending",
    amountUnits: 50000,
    ...USD,
    createdAt: "2026-07-24T13:57:19Z",
    idempotencyKey: "top_7d0e5c93",
    postings: [
      {
        account: "assets/custody/psp/stripe",
        direction: "debit",
        amountUnits: 50000,
        ...USD,
      },
      {
        account: "liabilities/customers/cus_a1b9/wallet",
        direction: "credit",
        amountUnits: 48550,
        ...USD,
      },
      {
        account: "revenue/fees/card",
        direction: "credit",
        amountUnits: 1450,
        ...USD,
      },
    ],
  },
  {
    id: "txn_01J8ZC3H7T6V",
    ledger: "treasury",
    description: "FX conversion USD → MXN",
    status: "posted",
    amountUnits: 1850000,
    ...USD,
    createdAt: "2026-07-24T13:22:54Z",
    idempotencyKey: "fx_c3391e08",
    postings: [
      {
        account: "assets/fx/clearing",
        direction: "debit",
        amountUnits: 1850000,
        ...USD,
      },
      {
        account: "assets/fx/usd-pool",
        direction: "credit",
        amountUnits: 1850000,
        ...USD,
      },
      {
        account: "assets/fx/mxn-pool",
        direction: "debit",
        amountUnits: 33689500,
        ...MXN,
      },
      {
        account: "assets/fx/clearing",
        direction: "credit",
        amountUnits: 33689500,
        ...MXN,
      },
    ],
  },
  {
    id: "txn_01J8ZC1F4K8D",
    ledger: "main",
    description: "Refund — duplicate charge",
    status: "reversed",
    amountUnits: 12999,
    ...USD,
    createdAt: "2026-07-24T12:48:03Z",
    idempotencyKey: "ref_5510fb2e",
    postings: [
      {
        account: "liabilities/customers/cus_90ee/wallet",
        direction: "debit",
        amountUnits: 12999,
        ...USD,
      },
      {
        account: "assets/custody/psp/stripe",
        direction: "credit",
        amountUnits: 12999,
        ...USD,
      },
    ],
  },
  {
    id: "txn_01J8ZBYD2R5Q",
    ledger: "main",
    description: "SEPA deposit — EUR corporate account",
    status: "posted",
    amountUnits: 750000,
    ...EUR,
    createdAt: "2026-07-24T11:35:47Z",
    idempotencyKey: "dep_e88c204b",
    postings: [
      {
        account: "assets/custody/bank/bbva-eu",
        direction: "debit",
        amountUnits: 750000,
        ...EUR,
      },
      {
        account: "liabilities/customers/cus_44d0/wallet",
        direction: "credit",
        amountUnits: 748125,
        ...EUR,
      },
      {
        account: "revenue/fees/deposit",
        direction: "credit",
        amountUnits: 1875,
        ...EUR,
      },
    ],
  },
  {
    id: "txn_01J8ZBW93M1Z",
    ledger: "main",
    description: "Internal wallet transfer",
    status: "posted",
    amountUnits: 89900,
    ...USD,
    createdAt: "2026-07-24T10:12:30Z",
    idempotencyKey: "wtr_1a6f77c5",
    postings: [
      {
        account: "liabilities/customers/cus_8c31/wallet",
        direction: "debit",
        amountUnits: 89900,
        ...USD,
      },
      {
        account: "liabilities/customers/cus_2f77/wallet",
        direction: "credit",
        amountUnits: 89900,
        ...USD,
      },
    ],
  },
  {
    id: "txn_01J8ZBT57C9N",
    ledger: "main",
    description: "Hold captured — marketplace escrow",
    status: "posted",
    amountUnits: 425000,
    ...USD,
    createdAt: "2026-07-24T09:03:12Z",
    idempotencyKey: "cap_b2d94310",
    postings: [
      {
        account: "liabilities/customers/cus_71aa/holds",
        direction: "debit",
        amountUnits: 425000,
        ...USD,
      },
      {
        account: "liabilities/merchants/mer_305f/wallet",
        direction: "credit",
        amountUnits: 412250,
        ...USD,
      },
      {
        account: "revenue/fees/escrow",
        direction: "credit",
        amountUnits: 12750,
        ...USD,
      },
    ],
  },
];

export const mockDashboard: DashboardData = {
  stats: {
    custodyUnits: 12845003217,
    custodyAsset: "USD",
    custodyExponent: 2,
    transactionsToday: 18942,
    openDiscrepancies: 7,
    unbalancedProviders: 2,
  },
  recentTransactions: mockTransactions.slice(0, 6),
  reconHealth: [
    {
      source: "bank_bofa",
      label: "Bank of America (custodia)",
      matchedPct: 99.98,
      openDiscrepancies: 1,
      lastRunAt: "2026-07-24T14:00:00Z",
      status: "healthy",
    },
    {
      source: "psp_stripe",
      label: "Stripe (PSP tarjetas)",
      matchedPct: 99.71,
      openDiscrepancies: 2,
      lastRunAt: "2026-07-24T14:00:00Z",
      status: "healthy",
    },
    {
      source: "provider_acmepay",
      label: "AcmePay (payouts)",
      matchedPct: 97.42,
      openDiscrepancies: 3,
      lastRunAt: "2026-07-24T13:30:00Z",
      status: "degraded",
    },
    {
      source: "provider_nordpay",
      label: "NordPay (pay-in LatAm)",
      matchedPct: 94.86,
      openDiscrepancies: 1,
      lastRunAt: "2026-07-24T11:00:00Z",
      status: "critical",
    },
  ],
};

export const mockLedgers: LedgerSummary[] = [
  {
    id: "main",
    name: "main",
    description: "Ledger operativo — custodia de clientes y fees",
    accounts: [
      {
        path: "assets/custody/bank/bofa",
        balances: [{ ...USD, units: 8412050000 }],
      },
      {
        path: "assets/custody/bank/bbva-eu",
        balances: [{ ...EUR, units: 1290300550 }],
      },
      {
        path: "assets/custody/psp/stripe",
        balances: [{ ...USD, units: 1230025055 }],
      },
      {
        path: "assets/custody/provider/acmepay",
        balances: [{ ...USD, units: 486120033 }],
      },
      {
        path: "liabilities/customers/wallets",
        balances: [
          { ...USD, units: 9822415088 },
          { ...EUR, units: 1268301550 },
        ],
      },
      {
        path: "liabilities/customers/holds",
        balances: [{ ...USD, units: 158204500 }],
      },
      {
        path: "liabilities/merchants/wallets",
        balances: [{ ...USD, units: 301270444 }],
      },
      {
        path: "revenue/fees/deposit",
        balances: [
          { ...USD, units: 45120033 },
          { ...EUR, units: 3801275 },
        ],
      },
      {
        path: "revenue/fees/payout",
        balances: [{ ...USD, units: 61208419 }],
      },
      {
        path: "revenue/fees/escrow",
        balances: [{ ...USD, units: 9410275 }],
      },
    ],
  },
  {
    id: "treasury",
    name: "treasury",
    description: "Ledger de tesorería — pools FX y capital propio",
    accounts: [
      {
        path: "assets/fx/usd-pool",
        balances: [{ ...USD, units: 2500000000 }],
      },
      {
        path: "assets/fx/mxn-pool",
        balances: [{ ...MXN, units: 41258700900 }],
      },
      {
        path: "assets/fx/clearing",
        balances: [
          { ...USD, units: 0 },
          { ...MXN, units: 0 },
        ],
      },
      {
        path: "equity/operating-capital",
        balances: [{ ...USD, units: 5000000000 }],
      },
    ],
  },
];

export const mockReconciliation: ReconciliationData = {
  runs: [
    {
      id: "run_01J8ZCE2M7K1",
      source: "bank_bofa",
      windowStart: "2026-07-24T13:00:00Z",
      windowEnd: "2026-07-24T14:00:00Z",
      status: "completed",
      matched: 4218,
      unmatched: 1,
    },
    {
      id: "run_01J8ZCD8Q2P5",
      source: "psp_stripe",
      windowStart: "2026-07-24T13:00:00Z",
      windowEnd: "2026-07-24T14:00:00Z",
      status: "completed",
      matched: 7301,
      unmatched: 2,
    },
    {
      id: "run_01J8ZCC1H9T4",
      source: "provider_acmepay",
      windowStart: "2026-07-24T12:30:00Z",
      windowEnd: "2026-07-24T13:30:00Z",
      status: "completed",
      matched: 1874,
      unmatched: 3,
    },
    {
      id: "run_01J8ZCB6X3R8",
      source: "provider_nordpay",
      windowStart: "2026-07-24T10:00:00Z",
      windowEnd: "2026-07-24T11:00:00Z",
      status: "failed",
      matched: 903,
      unmatched: 1,
    },
    {
      id: "run_01J8ZCA0V5N2",
      source: "provider_nordpay",
      windowStart: "2026-07-24T14:00:00Z",
      windowEnd: "2026-07-24T15:00:00Z",
      status: "running",
      matched: 412,
      unmatched: 0,
    },
  ],
  discrepancies: [
    {
      id: "dsc_01J8ZCH9W2M6",
      source: "provider_acmepay",
      type: "amount_mismatch",
      status: "open",
      externalRef: "THN-8842107",
      amountUnits: 2650,
      asset: "USD",
      exponent: 2,
      detectedAt: "2026-07-24T13:31:02Z",
    },
    {
      id: "dsc_01J8ZCG4T8K3",
      source: "provider_acmepay",
      type: "missing_internal",
      status: "investigating",
      externalRef: "THN-8841990",
      amountUnits: 125000,
      asset: "USD",
      exponent: 2,
      detectedAt: "2026-07-24T13:30:47Z",
    },
    {
      id: "dsc_01J8ZCF7R1Q9",
      source: "psp_stripe",
      type: "duplicate",
      status: "open",
      externalRef: "ch_3PqL8w2e",
      amountUnits: 12999,
      asset: "USD",
      exponent: 2,
      detectedAt: "2026-07-24T14:01:12Z",
    },
    {
      id: "dsc_01J8ZCE9N4V7",
      source: "psp_stripe",
      type: "missing_external",
      status: "investigating",
      externalRef: "txn_01J8ZBW93M1Z",
      amountUnits: 48550,
      asset: "USD",
      exponent: 2,
      detectedAt: "2026-07-24T14:00:58Z",
    },
    {
      id: "dsc_01J8ZCD2K6H1",
      source: "provider_nordpay",
      type: "missing_internal",
      status: "open",
      externalRef: "DL-20260724-3312",
      amountUnits: 890000,
      asset: "MXN",
      exponent: 2,
      detectedAt: "2026-07-24T11:02:33Z",
    },
    {
      id: "dsc_01J8ZCC5J9B8",
      source: "bank_bofa",
      type: "amount_mismatch",
      status: "resolved",
      externalRef: "BOFA-ACH-99120",
      amountUnits: 100,
      asset: "USD",
      exponent: 2,
      detectedAt: "2026-07-23T22:14:09Z",
    },
    {
      id: "dsc_01J8ZCB8G3F2",
      source: "provider_acmepay",
      type: "amount_mismatch",
      status: "open",
      externalRef: "THN-8839455",
      amountUnits: 410,
      asset: "USD",
      exponent: 2,
      detectedAt: "2026-07-24T12:30:15Z",
    },
  ],
};

export const mockDevelopers: DeveloperData = {
  apiKeys: [
    {
      id: "key_01J8Z9A1B2C3",
      name: "production-backend",
      prefix: "lc_live_8fk2…",
      createdAt: "2026-05-12T09:15:00Z",
      lastUsedAt: "2026-07-24T14:29:51Z",
    },
    {
      id: "key_01J8Z9D4E5F6",
      name: "recon-worker",
      prefix: "lc_live_2mq7…",
      createdAt: "2026-06-03T16:42:00Z",
      lastUsedAt: "2026-07-24T14:00:03Z",
    },
    {
      id: "key_01J8Z9G7H8J9",
      name: "sandbox-testing",
      prefix: "lc_test_5rt0…",
      createdAt: "2026-07-01T11:08:00Z",
      lastUsedAt: null,
    },
  ],
  webhooks: [
    {
      id: "whk_01J8ZA1K2L3M",
      url: "https://api.acme-fintech.com/hooks/ledgercore",
      topics: ["ledger.transaction.posted", "ledger.transaction.reversed"],
      status: "active",
      createdAt: "2026-05-12T09:30:00Z",
    },
    {
      id: "whk_01J8ZA4N5P6Q",
      url: "https://ops.acme-fintech.com/hooks/recon",
      topics: ["recon.discrepancy.detected"],
      status: "active",
      createdAt: "2026-06-10T14:20:00Z",
    },
    {
      id: "whk_01J8ZA7R8S9T",
      url: "https://staging.acme-fintech.com/hooks/all",
      topics: [
        "ledger.hold.created",
        "ledger.hold.captured",
        "ledger.hold.released",
      ],
      status: "paused",
      createdAt: "2026-07-02T08:05:00Z",
    },
  ],
};
