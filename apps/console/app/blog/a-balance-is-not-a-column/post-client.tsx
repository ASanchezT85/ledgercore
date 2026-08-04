"use client";

import { BlogPostShell, PostBody } from "@/components/blog-post-shell";
import { Code, CodeBlock, H2, P, Strong, UL } from "@/components/prose";

const SQL_DRIFT = `-- The query that should return zero rows, forever.
SELECT a.id,
       a.balance                AS stored,
       COALESCE(SUM(p.amount),0) AS derived
FROM accounts a
LEFT JOIN postings p ON p.account_id = a.id
GROUP BY a.id, a.balance
HAVING a.balance <> COALESCE(SUM(p.amount), 0);`;

const SQL_ATOMIC = `BEGIN;
  INSERT INTO postings (tx_id, account_id, amount, asset) VALUES ...;

  -- Same transaction. Not a job, not a trigger on another connection,
  -- not "eventually".
  UPDATE accounts
     SET balance = balance + $delta
   WHERE id = $account_id
     AND balance + $delta >= 0;   -- overdraft check, race-free

  -- 0 rows updated => insufficient funds => the whole thing rolls back.
COMMIT;`;

function BodyEs() {
  return (
    <>
      <P>
        Hay un momento en la vida de todo sistema de dinero en el que alguien
        mira el perfil de una query, ve que calcular el saldo suma diez mil
        filas, y agrega una columna <Code>balance</Code>. Es la decisión
        correcta. También es el momento en que pasás a tener{" "}
        <em className="text-ink">dos</em> fuentes de verdad.
      </P>
      <P>
        El problema no es la columna. El problema es no decidir explícitamente
        cuál de las dos manda.
      </P>

      <H2>Las dos verdades</H2>
      <P>
        Después de agregar la columna tenés un saldo{" "}
        <Strong>derivado</Strong> (la suma de los asientos) y un saldo{" "}
        <Strong>almacenado</Strong> (la columna). Mientras coincidan, nadie
        nota nada. El día que se separan, la pregunta operativa no es «¿cuál
        es el saldo?» sino «¿desde cuándo?», y esa es una pregunta mucho más
        cara.
      </P>
      <P>
        Vi esta separación en producción. La columna se actualizaba en un
        camino de código; un flujo nuevo insertaba movimientos por otro y se
        olvidaba de la columna. Nada falló. Nada alertó. Simplemente, a partir
        de una fecha, los dos números empezaron a contar historias distintas —
        y como los reportes leían la columna y el soporte leía los
        movimientos, cada área tenía razón.
      </P>

      <H2>La regla: derivado manda, almacenado es caché</H2>
      <P>
        La suma de los asientos es la verdad. Siempre. La columna es un caché
        de esa verdad, y como todo caché, necesita dos cosas: que se actualice
        atómicamente y que alguien verifique que no mintió.
      </P>
      <P>
        <Strong>Atómico</Strong> significa en la misma transacción de base de
        datos que el asiento. No en un job, no en un trigger sobre otra
        conexión, no «eventualmente»:
      </P>
      <CodeBlock code={SQL_ATOMIC} />
      <P>
        Ese <Code>AND balance + $delta &gt;= 0</Code> hace doble trabajo:
        materializa el saldo y aplica el chequeo de sobregiro sin carrera,
        porque el UPDATE toma el lock de la fila. Dos requests concurrentes no
        pueden leer el mismo disponible y gastarlo los dos; el segundo espera
        y ve el saldo ya movido.
      </P>

      <H2>Y la verificación, que es la parte que se saltea</H2>
      <P>
        La atomicidad te protege del código que escribiste bien. La
        verificación te protege del resto: la migración que tocó filas a mano,
        el script de arreglo de un incidente, el flujo nuevo del trimestre que
        viene.
      </P>
      <CodeBlock code={SQL_DRIFT} />
      <P>
        Es una query aburrida que tiene que devolver cero filas siempre.
        Corrida como chequeo periódico, convierte «alguien lo va a notar en el
        cierre» en «me enteré en una hora». Y cuando devuelve algo, te devuelve
        la cuenta exacta — no un total agregado que después hay que perseguir.
      </P>

      <H2>Lo que esto te habilita</H2>
      <UL>
        <li>
          <Strong>Saldo a cualquier fecha de corte.</Strong> Si el derivado es
          la verdad, el saldo del 31 de marzo es la misma suma con un{" "}
          <Code>WHERE posted_at &lt;= …</Code>. Sin snapshots, sin tabla de
          cierres mensuales.
        </li>
        <li>
          <Strong>Reconstrucción.</Strong> Si la columna se corrompe, se
          recalcula. Al revés no funciona: de una columna no se derivan los
          asientos que la produjeron.
        </li>
        <li>
          <Strong>Auditoría barata.</Strong> «¿Por qué este saldo es este?» se
          responde listando los asientos, no reconstruyendo intenciones.
        </li>
      </UL>

      <P>
        Todo esto se apoya en una condición que conviene decir en voz alta:
        los asientos tienen que ser inmutables. Si se pueden editar, el
        derivado también miente, y entonces no tenés dos fuentes de verdad —
        tenés cero.
      </P>
      <P>
        En LedgerCore el saldo se materializa en la misma transacción que el
        posting y el chequeo de arriba corre como invariante. Si estás
        leyendo esto con una columna <Code>balance</Code> en producción, la
        query de la deriva tarda un minuto en escribirse. Vale la pena saber
        la respuesta antes que el auditor.
      </P>
    </>
  );
}

function BodyEn() {
  return (
    <>
      <P>
        There is a moment in the life of every money system when someone
        profiles a query, sees that computing a balance sums ten thousand
        rows, and adds a <Code>balance</Code> column. That is the right call.
        It is also the moment you start having <em className="text-ink">two</em>{" "}
        sources of truth.
      </P>
      <P>
        The problem isn&apos;t the column. The problem is not deciding,
        explicitly, which of the two wins.
      </P>

      <H2>The two truths</H2>
      <P>
        After adding the column you have a <Strong>derived</Strong> balance
        (the sum of the entries) and a <Strong>stored</Strong> balance (the
        column). While they agree, nobody notices. The day they diverge, the
        operational question isn&apos;t &ldquo;what is the balance?&rdquo; but
        &ldquo;since when?&rdquo; — and that question is far more expensive.
      </P>
      <P>
        I&apos;ve watched this divergence happen in production. The column was
        updated on one code path; a newer flow inserted movements down another
        and forgot the column. Nothing failed. Nothing alerted. From one date
        onward the two numbers simply started telling different stories — and
        since reports read the column while support read the movements, every
        team was right.
      </P>

      <H2>The rule: derived wins, stored is a cache</H2>
      <P>
        The sum of the entries is the truth. Always. The column is a cache of
        that truth, and like every cache it needs two things: to be updated
        atomically, and to be checked for lying.
      </P>
      <P>
        <Strong>Atomic</Strong> means in the same database transaction as the
        entry. Not in a job, not in a trigger on another connection, not
        &ldquo;eventually&rdquo;:
      </P>
      <CodeBlock code={SQL_ATOMIC} />
      <P>
        That <Code>AND balance + $delta &gt;= 0</Code> does double duty: it
        materializes the balance and enforces the overdraft check without a
        race, because the UPDATE takes the row lock. Two concurrent requests
        cannot read the same available figure and both spend it; the second
        one waits and sees the balance already moved.
      </P>

      <H2>And the verification, which is the part people skip</H2>
      <P>
        Atomicity protects you from the code you wrote correctly. Verification
        protects you from everything else: the migration that touched rows by
        hand, the incident repair script, next quarter&apos;s new flow.
      </P>
      <CodeBlock code={SQL_DRIFT} />
      <P>
        It&apos;s a boring query that must return zero rows, forever. Run as a
        periodic check, it turns &ldquo;someone will notice at close&rdquo;
        into &ldquo;I knew within the hour.&rdquo; And when it does return
        something, it hands you the exact account — not an aggregate total you
        then have to chase.
      </P>

      <H2>What this buys you</H2>
      <UL>
        <li>
          <Strong>Balance as of any cutoff.</Strong> If derived is the truth,
          the March 31 balance is the same sum with a{" "}
          <Code>WHERE posted_at &lt;= …</Code>. No snapshots, no month-end
          table.
        </li>
        <li>
          <Strong>Reconstruction.</Strong> If the column is corrupted, you
          recompute it. The reverse doesn&apos;t work: a column cannot
          reproduce the entries that made it.
        </li>
        <li>
          <Strong>Cheap audits.</Strong> &ldquo;Why is this balance this
          number?&rdquo; is answered by listing entries, not by reconstructing
          intent.
        </li>
      </UL>

      <P>
        All of this rests on one condition worth saying out loud: the entries
        must be immutable. If they can be edited, the derived number lies too
        — and then you don&apos;t have two sources of truth, you have zero.
      </P>
      <P>
        In LedgerCore the balance is materialized in the same transaction as
        the posting, and the check above runs as an invariant. If you&apos;re
        reading this with a <Code>balance</Code> column in production, the
        drift query takes a minute to write. Worth knowing the answer before
        the auditor does.
      </P>
    </>
  );
}

export function BalanceColumnPostClient() {
  return (
    <BlogPostShell slug="a-balance-is-not-a-column">
      <PostBody es={<BodyEs />} en={<BodyEn />} />
    </BlogPostShell>
  );
}
