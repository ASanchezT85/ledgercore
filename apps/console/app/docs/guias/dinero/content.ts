import type { GuideContent } from "../../docs-ui";

const JSON_MONEY = `{
  "amount": { "asset": "USD", "amount": "10050" }
}`;

const FLOAT_TRAP = `> 0.1 + 0.2
0.30000000000000004

> 100.35 * 100
10034.999999999998`;

const TS_MONEY = `import { Money } from "@ledgercore/sdk";

Money.fromDecimal("100.50", "USD", 2); // { asset: "USD", amount: "10050" }
Money.toDecimal("10050", 2);           // "100.50"

// fromDecimal LANZA si el decimal excede el exponente:
Money.fromDecimal("100.505", "USD", 2); // Error — nunca redondea en silencio`;

const PHP_MONEY = `use LedgerCore\\Money;

Money::fromDecimal('100.50', 'USD', 2); // ['asset' => 'USD', 'amount' => '10050']
Money::toDecimal('10050', 2);           // "100.50"

// fromDecimal LANZA si el decimal excede el exponente:
Money::fromDecimal('100.505', 'USD', 2); // excepción — sin redondeo silencioso`;

const POSTING_EXAMPLE = `{
  "postings": [
    { "account_id": "…cash…",   "direction": "DEBIT",  "amount": { "asset": "USD", "amount": "10000" } },
    { "account_id": "…wallet…", "direction": "CREDIT", "amount": { "asset": "USD", "amount": "9700" } },
    { "account_id": "…fees…",   "direction": "CREDIT", "amount": { "asset": "USD", "amount": "300" } }
  ]
}`;

export const GUIDE: GuideContent = {
  es: {
    badge: "Guía · Dinero y montos",
    title: "Dinero: enteros en unidades menores, siempre strings",
    subtitle:
      "Todos los montos de la API son enteros en la unidad menor del activo (centavos para USD), codificados como strings en JSON. Nunca floats, nunca decimales: es la única forma de que un ledger cuadre al centavo.",
    backLabel: "Docs",
    copy: "Copiar",
    copied: "Copiado",
    sections: [
      {
        title: "El formato: { asset, amount }",
        body: "Cada monto viaja como un objeto con el código del activo y la cantidad en unidades menores. $100.50 USD son 10050 centavos. El exponente del activo (2 para USD) define cuántos decimales representa la unidad menor.",
        blocks: [{ kind: "resp", label: "Un monto en el JSON de la API", code: JSON_MONEY }],
      },
      {
        title: "Por qué strings y no números JSON",
        body: "Los montos son int64. JSON no tiene enteros de 64 bits: los parsers de JavaScript convierten los números a float64 y pierden precisión a partir de 2^53. String-encoded, el monto llega intacto a cualquier lenguaje. Es la misma razón por la que los ids de Twitter/Stripe viajan como strings.",
      },
      {
        title: "Por qué nunca floats",
        body: "La aritmética binaria de punto flotante no representa exactamente los decimales: los errores se acumulan y el ledger deja de cuadrar. Un descuadre de un centavo en un ledger append-only no se corrige — se arrastra. Si no balancea, no se postea.",
        blocks: [{ kind: "resp", label: "La trampa del float (consola de JS)", code: FLOAT_TRAP }],
      },
      {
        title: "Money helpers en TypeScript",
        body: "El SDK convierte entre decimal humano y unidades menores. fromDecimal lanza si el decimal tiene más cifras que el exponente: el dinero no se redondea en silencio.",
        blocks: [{ kind: "code", label: "TypeScript · Money", code: TS_MONEY }],
      },
      {
        title: "Money helpers en PHP",
        body: "Idéntica semántica en PHP: strings de entrada, strings de salida, error explícito ante precisión imposible.",
        blocks: [{ kind: "code", label: "PHP · Money", code: PHP_MONEY }],
      },
      {
        title: "El invariante: débitos = créditos por asset",
        body: "Cada transacción debe balancear débitos y créditos por activo, en unidades menores exactas. 10000 = 9700 + 300. Si los apuntes no cuadran (o hay overflow monetario), la API responde 422 unbalanced_transaction y no postea nada.",
        blocks: [{ kind: "resp", label: "Postings balanceados (100 / 97 / 3)", code: POSTING_EXAMPLE }],
      },
    ],
  },
  en: {
    badge: "Guide · Money and amounts",
    title: "Money: integers in minor units, always strings",
    subtitle:
      "Every amount in the API is an integer in the asset's minor unit (cents for USD), string-encoded in JSON. Never floats, never decimals: it is the only way a ledger balances to the cent.",
    backLabel: "Docs",
    copy: "Copy",
    copied: "Copied",
    sections: [
      {
        title: "The shape: { asset, amount }",
        body: "Every amount travels as an object with the asset code and the quantity in minor units. $100.50 USD is 10050 cents. The asset's exponent (2 for USD) defines how many decimals the minor unit represents.",
        blocks: [{ kind: "resp", label: "An amount in the API's JSON", code: JSON_MONEY }],
      },
      {
        title: "Why strings and not JSON numbers",
        body: "Amounts are int64. JSON has no 64-bit integers: JavaScript parsers coerce numbers to float64 and lose precision past 2^53. String-encoded, the amount arrives intact in every language. Same reason Twitter/Stripe ids travel as strings.",
      },
      {
        title: "Why never floats",
        body: "Binary floating-point arithmetic cannot represent decimals exactly: errors accumulate and the ledger stops balancing. A one-cent drift in an append-only ledger is never corrected — it compounds. If it doesn't balance, it doesn't post.",
        blocks: [{ kind: "resp", label: "The float trap (JS console)", code: FLOAT_TRAP }],
      },
      {
        title: "Money helpers in TypeScript",
        body: "The SDK converts between human decimals and minor units. fromDecimal throws when the decimal has more digits than the exponent: money is never silently rounded.",
        blocks: [{ kind: "code", label: "TypeScript · Money", code: TS_MONEY }],
      },
      {
        title: "Money helpers in PHP",
        body: "Identical semantics in PHP: strings in, strings out, explicit error on impossible precision.",
        blocks: [{ kind: "code", label: "PHP · Money", code: PHP_MONEY }],
      },
      {
        title: "The invariant: debits = credits per asset",
        body: "Every transaction must balance debits and credits per asset, in exact minor units. 10000 = 9700 + 300. If the postings don't add up (or a monetary overflow occurs), the API answers 422 unbalanced_transaction and posts nothing.",
        blocks: [{ kind: "resp", label: "Balanced postings (100 / 97 / 3)", code: POSTING_EXAMPLE }],
      },
    ],
  },
};
