"use client";

import { BlogPostShell, PostBody } from "@/components/blog-post-shell";
import { Code, CodeBlock, H2, P, Strong, UL } from "@/components/prose";

const SQL_KEYS = `-- The key is a UNIQUE constraint, not a lookup.
CREATE TABLE idempotency_keys (
    tenant_id     uuid        NOT NULL,
    key           text        NOT NULL,
    request_hash  text        NOT NULL,   -- so a reused key with a
                                          -- different body is rejected
    transaction_id uuid,                  -- filled once the work commits
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key)
);`;

const SQL_RACE = `BEGIN;
  -- Claim the key FIRST, in the same transaction as the money.
  INSERT INTO idempotency_keys (tenant_id, key, request_hash)
  VALUES ($1, $2, $3);          -- second concurrent retry blocks here,
                                -- then fails on the primary key

  INSERT INTO transactions ...  -- the postings
  UPDATE idempotency_keys SET transaction_id = ... WHERE ...;
COMMIT;`;

function BodyEs() {
  return (
    <>
      <P>
        Casi todas las APIs de pagos aceptan un header{" "}
        <Code>Idempotency-Key</Code>. Bastantes menos hacen algo con él. Y
        aceptar la llave sin cumplirla es peor que no aceptarla: el que
        integra lee tu documentación, ve el header, y deja de defenderse por
        su cuenta.
      </P>
      <P>
        La idempotencia no es un header. Es una promesa con tres partes, y si
        te falta una, la promesa no se sostiene.
      </P>

      <H2>Parte 1: el mismo comando dos veces produce un solo movimiento</H2>
      <P>
        Esta es la parte obvia y la única que la mayoría implementa. El
        segundo request con la misma llave no crea una transacción nueva.
      </P>
      <P>
        La implementación ingenua es «buscá la llave; si existe, devolvé
        temprano». Eso funciona en tu máquina y falla en producción, porque
        entre el SELECT y el INSERT hay una ventana en la que cabe
        perfectamente el segundo reintento.
      </P>

      <H2>Parte 2: el replay devuelve la respuesta original</H2>
      <P>
        Un <Code>200 OK</Code> vacío no sirve. El que reintenta lo hace porque
        no supo cómo terminó la primera vez — necesita el{" "}
        <em className="text-ink">mismo</em> cuerpo de respuesta: el mismo id
        de transacción, el mismo estado, los mismos montos.
      </P>
      <P>
        Si el replay devuelve algo distinto de la respuesta original, quien
        integra tiene que reconciliar dos verdades, que es exactamente el
        trabajo del que lo querías salvar.
      </P>

      <H2>Parte 3: la misma llave con otro cuerpo es un error, no un replay</H2>
      <P>
        Este es el que casi nadie hace. Si llega la llave{" "}
        <Code>dep_9f2c41d7</Code> con un monto de 100 y después la misma llave
        con un monto de 900, no es un reintento: es un bug del lado del
        cliente, o algo peor. Devolver alegremente la primera respuesta
        esconde el problema; devolver un <Code>409</Code> lo expone mientras
        todavía es barato.
      </P>
      <P>
        Por eso se guarda un hash del request junto a la llave. La llave dice
        «esto es el mismo comando»; el hash lo verifica.
      </P>
      <CodeBlock code={SQL_KEYS} />

      <H2>La carrera que rompe la implementación ingenua</H2>
      <P>
        Dos reintentos simultáneos — un cliente impaciente, o un proxy que
        reintenta mientras el original sigue en vuelo — llegan con
        milisegundos de diferencia. Ambos consultan la tabla de llaves, ambos
        no encuentran nada, ambos siguen. Dos depósitos. El cliente acreditado
        dos veces.
      </P>
      <P>
        <Strong>El arreglo no es un mutex de aplicación.</Strong> Es reclamar
        la llave en la misma transacción de base de datos que el dinero, y
        dejar que la clave primaria haga de árbitro:
      </P>
      <CodeBlock code={SQL_RACE} />
      <P>
        El segundo reintento se bloquea en el INSERT, espera al commit del
        primero, y falla con violación de unicidad. Ahí ya sabés que es un
        replay: leés la fila y devolvés la respuesta original. Un único árbitro
        —el que ya usás para el dinero— en lugar de un candado nuevo que
        también puede fallar.
      </P>

      <H2>Qué dejar fuera del alcance de la llave</H2>
      <UL>
        <li>
          <Strong>El tenant.</Strong> La llave es única por tenant, nunca
          global: dos clientes pueden elegir el mismo string sin colisionar.
        </li>
        <li>
          <Strong>El tiempo.</Strong> Una llave que expira a los 5 minutos es
          una llave que no protege del reintento que llega a los 6. Si vas a
          purgarlas, que sea en meses, y decilo en la documentación.
        </li>
        <li>
          <Strong>Las lecturas.</Strong> Un GET ya es idempotente; pedir la
          llave ahí solo agrega ruido.
        </li>
      </UL>

      <P>
        La prueba de que la promesa se cumple es aburrida y hay que escribirla
        igual: mandá el mismo comando N veces en paralelo, y afirmá que el
        saldo se movió exactamente una vez. Si ese test no existe, la
        idempotencia de tu API es una intención documentada.
      </P>
      <P>
        En LedgerCore la llave es parte del contrato de escritura, con esas
        tres partes, y hay un test de concurrencia que lo sostiene. Lo cuento
        porque es la clase de cosa que se descubre tarde: normalmente, el día
        que un proveedor reintenta un callback que vos ya habías respondido.
      </P>
    </>
  );
}

function BodyEn() {
  return (
    <>
      <P>
        Almost every payments API accepts an <Code>Idempotency-Key</Code>{" "}
        header. Considerably fewer do anything with it. And accepting the key
        without honoring it is worse than not accepting one: the integrator
        reads your docs, sees the header, and stops defending themselves.
      </P>
      <P>
        Idempotency is not a header. It&apos;s a promise with three parts, and
        if you&apos;re missing one, the promise doesn&apos;t hold.
      </P>

      <H2>Part 1: the same command twice produces one movement</H2>
      <P>
        This is the obvious part, and the only one most implementations get
        to. The second request with the same key does not create a new
        transaction.
      </P>
      <P>
        The naive implementation is &ldquo;look the key up; if it exists,
        return early.&rdquo; That works on your laptop and fails in
        production, because between the SELECT and the INSERT there is a
        window that fits the second retry perfectly.
      </P>

      <H2>Part 2: the replay returns the original response</H2>
      <P>
        An empty <Code>200 OK</Code> is useless. Whoever is retrying is doing
        so because they never learned how the first attempt ended — they need
        the <em className="text-ink">same</em> response body: same transaction
        id, same status, same amounts.
      </P>
      <P>
        If the replay returns something different from the original response,
        the integrator has to reconcile two truths, which is exactly the job
        you meant to save them from.
      </P>

      <H2>Part 3: same key, different body is an error, not a replay</H2>
      <P>
        This is the part almost nobody implements. If key{" "}
        <Code>dep_9f2c41d7</Code> arrives with an amount of 100, and then the
        same key arrives with an amount of 900, that is not a retry: it&apos;s
        a client-side bug, or something worse. Cheerfully returning the first
        response hides the problem; returning a <Code>409</Code> surfaces it
        while it&apos;s still cheap.
      </P>
      <P>
        That&apos;s why you store a hash of the request alongside the key. The
        key claims &ldquo;this is the same command&rdquo;; the hash verifies
        it.
      </P>
      <CodeBlock code={SQL_KEYS} />

      <H2>The race that breaks the naive implementation</H2>
      <P>
        Two simultaneous retries — an impatient client, or a proxy retrying
        while the original is still in flight — arrive milliseconds apart.
        Both query the keys table, both find nothing, both proceed. Two
        deposits. The customer credited twice.
      </P>
      <P>
        <Strong>The fix is not an application mutex.</Strong> It&apos;s
        claiming the key in the same database transaction as the money, and
        letting the primary key arbitrate:
      </P>
      <CodeBlock code={SQL_RACE} />
      <P>
        The second retry blocks on the INSERT, waits for the first to commit,
        and fails on the uniqueness violation. At that point you know it&apos;s
        a replay: read the row and return the original response. One arbiter —
        the one you already trust with the money — instead of a new lock that
        can also fail.
      </P>

      <H2>What to keep out of the key&apos;s scope</H2>
      <UL>
        <li>
          <Strong>The tenant.</Strong> The key is unique per tenant, never
          globally: two customers can pick the same string without colliding.
        </li>
        <li>
          <Strong>Time.</Strong> A key that expires after 5 minutes is a key
          that doesn&apos;t protect against the retry that lands at minute 6.
          If you purge them, purge in months — and say so in the docs.
        </li>
        <li>
          <Strong>Reads.</Strong> A GET is already idempotent; requiring the
          key there only adds noise.
        </li>
      </UL>

      <P>
        The test that proves the promise is boring, and you have to write it
        anyway: fire the same command N times in parallel and assert the
        balance moved exactly once. If that test doesn&apos;t exist, your
        API&apos;s idempotency is a documented intention.
      </P>
      <P>
        In LedgerCore the key is part of the write contract, with those three
        parts, and a concurrency test holds it up. I mention it because
        it&apos;s the kind of thing you discover late — usually the day a
        provider retries a callback you had already answered.
      </P>
    </>
  );
}

export function IdempotencyPostClient() {
  return (
    <BlogPostShell slug="idempotency-is-a-promise">
      <PostBody es={<BodyEs />} en={<BodyEn />} />
    </BlogPostShell>
  );
}
