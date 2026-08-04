"use client";

import { BlogPostShell, PostBody } from "@/components/blog-post-shell";
import { Code, CodeBlock, H2, P, Strong, UL } from "@/components/prose";

const SQL_HOLD = `-- A hold is a posting between two accounts the customer owns.
-- Nothing leaves the wallet; it moves to a sub-account they cannot spend.
INSERT INTO postings (tx_id, account, direction, amount, asset) VALUES
  ($tx, 'liabilities/customers/cus_8c31/available', 'debit',  50000, 'USD'),
  ($tx, 'liabilities/customers/cus_8c31/held',      'credit', 50000, 'USD');

-- Available balance is a balance, not an inference:
--   available = SUM(postings WHERE account = '.../available')`;

const SQL_STRANDED = `-- Holds with no live operation behind them.
SELECT h.id, h.account, h.amount, h.created_at, now() - h.created_at AS age
FROM holds h
LEFT JOIN operations o ON o.id = h.operation_id
                      AND o.status IN ('pending', 'processing')
WHERE h.status = 'active'
  AND o.id IS NULL;    -- the hold outlived its reason`;

function BodyEs() {
  return (
    <>
      <P>
        Un cliente empresa nos escribió porque su saldo disponible no le
        alcanzaba para una operación que claramente podía pagar. Miramos la
        wallet: cerca del 97% del saldo estaba retenido en reservas. Órdenes
        abiertas que lo justificaran: cero.
      </P>
      <P>
        La plata estaba. Nadie se la había llevado. Simplemente no había
        ningún camino de código capaz de devolvérsela.
      </P>

      <H2>Cómo se llega ahí</H2>
      <P>
        La forma más común de implementar una reserva es un campo: una
        columna <Code>is_reserved</Code>, un registro en una tabla lateral, un
        flag en el detalle de pago. Y la liberación se implementa como la
        contraparte de ese campo: cuando la operación termina, alguien lo
        apaga.
      </P>
      <P>
        Eso funciona mientras todos los flujos escriban el mismo campo. En
        nuestro caso, la liberación decidía «esta orden usó la wallet»
        mirando un dato colateral en un registro de detalle de pago. Un flujo
        más nuevo, escrito un año después, no creaba ese registro. Las órdenes
        se completaban perfecto. La liberación no encontraba nada que liberar.
        Las reservas se acumulaban para siempre.
      </P>
      <P>
        <Strong>La lección incómoda:</Strong> no fue un bug de la liberación.
        Fue un bug de haber definido «reservado» como un efecto lateral en vez
        de como un saldo.
      </P>

      <H2>Un hold es un movimiento, no un estado</H2>
      <P>
        La formulación que no se rompe es la contable: el dinero retenido no
        es dinero marcado, es dinero <em className="text-ink">movido</em>. El
        cliente tiene dos cuentas —disponible y retenido— y un hold es un
        asiento entre las dos.
      </P>
      <CodeBlock code={SQL_HOLD} />
      <P>
        Fijate lo que cambia. El saldo total del cliente no se movió: sigue
        siendo la suma de sus dos cuentas, y la doble entrada lo garantiza. Lo
        que cambió es <em className="text-ink">cuánto puede gastar</em>, y esa
        pregunta ahora se responde leyendo un saldo, no cruzando tablas
        laterales.
      </P>
      <P>
        Una liberación es el asiento inverso. Una captura es un asiento de la
        cuenta retenida hacia afuera. Un vencimiento es una liberación con
        otro motivo. Tres operaciones distintas, un solo mecanismo, y ninguna
        de ellas puede dejar plata en un limbo que no sea un saldo consultable.
      </P>

      <H2>Los holds necesitan un vencimiento</H2>
      <P>
        Un hold sin fecha de expiración es una promesa sin vencimiento, y las
        promesas sin vencimiento se acumulan. Cada reserva se crea con un{" "}
        <Code>expires_at</Code>, y hay un proceso que libera lo vencido — con
        su asiento, visible, no un UPDATE silencioso.
      </P>
      <P>
        Eso no alcanza. También hace falta la query que busca reservas activas
        cuya operación ya no existe o ya terminó:
      </P>
      <CodeBlock code={SQL_STRANDED} />
      <P>
        Esa consulta es la que nos hubiera avisado en un día en lugar de en
        semanas. Y ojo con el detalle: tiene que partir{" "}
        <em className="text-ink">del hold hacia la operación</em>. La versión
        que parte de las operaciones para ver si les falta liberar nunca
        encuentra las reservas huérfanas, porque la operación que las dejó
        colgadas ya no está en la lista.
      </P>

      <H2>La lista corta</H2>
      <UL>
        <li>
          El disponible es un saldo derivado de asientos, no un total menos un
          flag.
        </li>
        <li>
          Reservar, liberar, capturar y vencer son todos asientos. Ninguno es
          un UPDATE de estado.
        </li>
        <li>
          Todo hold nace con vencimiento, y el vencimiento se ejecuta con su
          propio asiento.
        </li>
        <li>
          El chequeo de huérfanos se escribe desde el hold, nunca desde la
          operación.
        </li>
      </UL>

      <P>
        En LedgerCore los holds son ciudadanos de primera: se postean, expiran
        y se liberan con el mismo rigor que cualquier otro movimiento, y el
        disponible siempre es posteado menos retenido vigente. No es
        sofisticado. Es, simplemente, lo que hubiera evitado que un cliente
        pasara semanas sin poder usar su propia plata.
      </P>
    </>
  );
}

function BodyEn() {
  return (
    <>
      <P>
        A business customer wrote in because their available balance
        wasn&apos;t enough for an operation they could clearly afford. We
        looked at the wallet: roughly 97% of the balance was locked in
        reserves. Open orders justifying it: zero.
      </P>
      <P>
        The money was there. Nobody had taken it. There simply was no code
        path capable of giving it back.
      </P>

      <H2>How you get there</H2>
      <P>
        The most common way to implement a reservation is a field: an{" "}
        <Code>is_reserved</Code> column, a row in a side table, a flag on the
        payment detail. And the release is implemented as that field&apos;s
        counterpart: when the operation finishes, someone turns it off.
      </P>
      <P>
        That works as long as every flow writes the same field. In our case
        the release decided &ldquo;this order used the wallet&rdquo; by
        reading a collateral value on a payment-detail record. A newer flow,
        written a year later, never created that record. Orders completed
        perfectly. The release found nothing to release. Reserves accumulated
        forever.
      </P>
      <P>
        <Strong>The uncomfortable lesson:</Strong> it was not a bug in the
        release. It was a bug in having defined &ldquo;reserved&rdquo; as a
        side effect instead of as a balance.
      </P>

      <H2>A hold is a movement, not a state</H2>
      <P>
        The formulation that doesn&apos;t break is the accounting one: held
        money is not flagged money, it&apos;s{" "}
        <em className="text-ink">moved</em> money. The customer has two
        accounts — available and held — and a hold is an entry between them.
      </P>
      <CodeBlock code={SQL_HOLD} />
      <P>
        Notice what changes. The customer&apos;s total balance didn&apos;t
        move: it&apos;s still the sum of their two accounts, and double-entry
        guarantees it. What changed is{" "}
        <em className="text-ink">how much they can spend</em>, and that
        question is now answered by reading a balance, not by joining side
        tables.
      </P>
      <P>
        A release is the inverse entry. A capture is an entry out of the held
        account. An expiry is a release with a different reason. Three
        distinct operations, one mechanism — and none of them can leave money
        in a limbo that isn&apos;t a queryable balance.
      </P>

      <H2>Holds need an expiry</H2>
      <P>
        A hold with no expiry is a promise with no deadline, and promises with
        no deadline pile up. Every reservation is created with an{" "}
        <Code>expires_at</Code>, and a process releases what has lapsed — with
        its entry, visible, not a silent UPDATE.
      </P>
      <P>
        That is not enough on its own. You also need the query that looks for
        active reservations whose operation no longer exists or has already
        finished:
      </P>
      <CodeBlock code={SQL_STRANDED} />
      <P>
        That query is what would have told us in a day instead of in weeks.
        And mind the detail: it has to start{" "}
        <em className="text-ink">from the hold and look for the operation</em>.
        The version that starts from operations and checks whether they forgot
        to release never finds the orphans, because the operation that
        stranded them isn&apos;t in the list anymore.
      </P>

      <H2>The short list</H2>
      <UL>
        <li>
          Available is a balance derived from entries, not a total minus a
          flag.
        </li>
        <li>
          Reserve, release, capture and expire are all entries. None of them
          is a status UPDATE.
        </li>
        <li>
          Every hold is born with an expiry, and expiring posts its own entry.
        </li>
        <li>
          The orphan check is written from the hold, never from the operation.
        </li>
      </UL>

      <P>
        In LedgerCore holds are first-class: posted, expired and released with
        the same rigor as any other movement, and available is always posted
        minus live holds. It isn&apos;t sophisticated. It&apos;s simply what
        would have kept a customer from spending weeks unable to use their own
        money.
      </P>
    </>
  );
}

export function HoldsPostClient() {
  return (
    <BlogPostShell slug="holds-are-money">
      <PostBody es={<BodyEs />} en={<BodyEn />} />
    </BlogPostShell>
  );
}
