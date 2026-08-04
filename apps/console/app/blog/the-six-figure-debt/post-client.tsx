"use client";

import { BlogPostShell, PostBody } from "@/components/blog-post-shell";
import { Code, CodeBlock, H2, P, Strong, UL } from "@/components/prose";

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

function BodyEs() {
  return (
    <>
      <P>
        Lo encontré un martes, cruzando el estado de liquidación de un
        proveedor de pagos contra nuestros propios libros. El proveedor decía
        que le debíamos seis cifras. Nuestros sistemas decían que no le
        debíamos nada. No «un número más chico» — nada. La deuda no existía en
        ningún lugar de la empresa.
      </P>
      <P>
        Pasé años operando infraestructura de movimiento de dinero para una
        plataforma de remesas. Volumen real, reguladores reales, socios de
        pago en varios países. Este post es sobre las cicatrices que me dejó
        esa operación y los principios de diseño que ya no negocio. Si estás
        construyendo algo que toca dinero sobre tablas de aplicación
        corrientes, parte de esto te va a resultar incómodamente familiar.
      </P>

      <H2>Por qué las tablas operativas + hojas de cálculo fallan por estructura</H2>
      <P>
        Casi todos los sistemas cerca de fintech empiezan igual: una tabla
        estilo <Code>orders</Code>, una columna <Code>balance</Code> en algún
        lado, y una constelación creciente de hojas de cálculo haciendo la
        conciliación por afuera. Funciona hasta que deja de funcionar, y la
        falla es estructural, no accidental:
      </P>
      <UL>
        <li>
          Las tablas operativas son mutables. Una fila que hoy dice
          «completada» mañana puede decir otra cosa, y nada registra la
          transición.
        </li>
        <li>
          Los saldos guardados como columna se separan de las transacciones
          que supuestamente los produjeron, porque las dos actualizaciones no
          son atómicas.
        </li>
        <li>
          La hoja de cálculo es la escotilla de escape, y la hoja de cálculo
          es hostil a la corrección: se han encontrado errores en cerca del
          94% de las hojas en uso, y alrededor del 52% de las empresas
          reportan excepciones materiales de conciliación en su cierre.
        </li>
      </UL>
      <P>
        Nada de eso pasa porque la gente sea descuidada. Pasa porque el modelo
        de datos no tiene el concepto de verdad financiera. Tiene el concepto
        de <em className="text-ink">estado actual</em>, que es otra cosa, más
        débil.
      </P>
      <P>
        Estas son las cinco cicatrices que me lo enseñaron, cada una con la
        lección que grabó.
      </P>

      <H2>Cicatriz 1: la deuda que no existía</H2>
      <P>
        Cuando por fin construimos un ledger de verdad, lo sembramos con un
        asiento de apertura derivado de los saldos actuales. La lógica de
        siembra tenía un supuesto silencioso: los saldos negativos eran ruido,
        así que los descartaba.
      </P>
      <P>
        Uno de esos valores «ruido» era nuestra posición con un proveedor de
        pagos. Teníamos un saldo de custodia{" "}
        <em className="text-ink">negativo</em> con ellos: habían pagado por
        nuestra cuenta más de lo que habíamos fondeado. Eso es un pasivo. Una
        deuda real, de seis cifras. El asiento de apertura la tiró, así que el
        ledger nació mintiendo, y todos los reportes aguas abajo heredaron la
        mentira.
      </P>
      <P>
        <Strong>Lección:</Strong> un saldo de custodia negativo es un pasivo,
        no una anomalía. Cualquier arranque de ledger que filtre por signo
        está eligiendo en silencio qué realidad conservar. La doble entrada
        hace ruidosa esta clase de bug: no podés descartar un lado de una
        posición sin que los libros dejen de balancear.
      </P>

      <H2>Cicatriz 2: la wallet congelada al 97%</H2>
      <P>
        La wallet de un cliente empresa mostraba ~97% de su saldo bloqueado en
        reservas — con cero transacciones abiertas. Semanas de holds, ninguna
        liberación.
      </P>
      <P>
        Causa raíz: la lógica de liberación detectaba «esta orden usó la
        wallet» mirando un campo colateral en un registro de detalle de pago.
        Un flujo de transacción más nuevo simplemente no creaba ese registro.
        Las órdenes se completaban bien; el chequeo de liberación no
        encontraba nada que liberar; las reservas se acumulaban para siempre.
      </P>
      <P>
        <Strong>Lección:</Strong> si una decisión que afecta dinero depende de
        la <em className="text-ink">forma incidental</em> de un dato
        operativo, cada flujo nuevo es una oportunidad de romperla en
        silencio. Los holds y las liberaciones tienen que ser asientos de
        primera clase: un hold es un posting, una liberación es un posting, y
        «cuánto hay reservado» es un saldo, no una inferencia sobre tablas
        laterales.
      </P>

      <H2>Cicatriz 3: saldos calculados sumando todo, cada vez</H2>
      <P>
        Los chequeos de saldo cargaban todas las filas de movimiento de una
        wallet y las sumaban en memoria. En cada request. Era lento, se ponía
        más lento linealmente con la historia, y peor: dos requests
        concurrentes podían leer el mismo «disponible» y gastarlo los dos.
      </P>
      <P>
        <Strong>Lección:</Strong> materializá los saldos en la{" "}
        <em className="text-ink">misma</em> transacción de base de datos que
        el asiento que los cambia. Las filas de postings siguen siendo la
        fuente de verdad; el saldo materializado es un caché al que{" "}
        <em className="text-ink">nunca se le permite desviarse</em>, porque se
        mueve atómicamente con los asientos. Obtenés lecturas O(1) y un lugar
        natural donde hacer cumplir las reglas de sobregiro.
      </P>

      <H2>Cicatriz 4: «1.500» se convirtió en 1500</H2>
      <P>
        Teníamos un helper que normalizaba montos decimales escritos por el
        usuario. Dado <Code>&quot;1.500&quot;</Code> decidía que el punto era
        separador de miles y devolvía 1500. Un error de ×1000 en un camino de
        dinero.
      </P>
      <P>
        Lo mejor: había un test unitario afirmando exactamente ese
        comportamiento. El bug estaba consagrado como especificación. Quien
        escribió el test miró la salida equivocada y la anotó como correcta.
      </P>
      <P>
        <Strong>Lección:</Strong> los floats y los parsers que olfatean el
        locale no tienen lugar cerca del dinero. Los montos son enteros en
        unidades menores, etiquetados con un código de activo y un exponente (
        <Code>amount: 150000, asset: &quot;USD&quot;, exponent: 2</Code>). El
        parseo pasa una sola vez, en el borde, contra un formato explícito —
        nunca adivinando qué significa un punto.
      </P>

      <H2>Cicatriz 5: semanas de SQL forense</H2>
      <P>
        Cada incidente de arriba terminó igual: yo, una réplica de lectura y
        SQL escrito a mano, reconstruyendo lo que{" "}
        <em className="text-ink">tuvo que haber pasado</em> a partir de filas
        mutables que solo guardaban cómo se veían las cosas{" "}
        <em className="text-ink">ahora</em>. Algunas preguntas eran
        directamente incontestables: los estados intermedios ya no estaban.
      </P>
      <P>
        <Strong>Lección:</Strong> si tu modelo de datos no puede reproducir la
        historia, cada incidente se vuelve arqueología. Append-only no es una
        preferencia de pureza; es la diferencia entre «consultar la línea de
        tiempo» y «entrevistar a los sobrevivientes».
      </P>

      <H2>Los principios que después metí en el ledger</H2>
      <P>
        No son aspiraciones. Cada uno es una cicatriz con la polaridad
        invertida.
      </P>
      <P>
        <Strong>1. Doble entrada, obligada, no sugerida.</Strong> Cada
        transacción es un conjunto de postings que suma cero por activo. No
        por convención: por chequeo, al momento de escribir.
      </P>
      <CodeBlock code={SQL_BALANCE_CHECK} />
      <P>
        Si esto hubiera existido el día uno, la Cicatriz 1 es imposible:
        descartar el lado pasivo del asiento de apertura no llega a commitear.
      </P>
      <P>
        <Strong>2. Append-only, con correcciones como asientos compensatorios.</Strong>{" "}
        Ni UPDATE ni DELETE en las tablas de postings — obligado en la propia
        base, para que ni un script de migración bienintencionado pueda
        reescribir la historia.
      </P>
      <CodeBlock code={SQL_APPEND_ONLY} />
      <P>
        ¿Te equivocaste? Posteá la reversa y el asiento corregido. El error
        queda visible. Esa visibilidad es la funcionalidad.
      </P>
      <P>
        <Strong>3. Dinero como enteros: activo + exponente.</Strong> Sin
        floats, sin strings, sin adivinar el locale. Debajo de la capa de
        presentación, 1500 USD es{" "}
        <Code>{"{amount: 150000, asset: \"USD\", exponent: 2}"}</Code> en todos
        lados. La Cicatriz 4 pasa a ser una pregunta de renderizado, no de
        solvencia.
      </P>
      <P>
        <Strong>4. Idempotencia de punta a punta.</Strong> Cada comando
        externo lleva una llave de idempotencia; los replays devuelven el
        resultado original en vez de postear dos veces. Los reintentos, las
        reentregas de webhooks y los clientes impacientes dejan de ser eventos
        de dinero.
      </P>
      <P>
        <Strong>5. Saldos materializados en la misma transacción.</Strong> El
        asiento y la actualización de saldo commitean juntos o no commitea
        ninguno. Las lecturas son O(1), los chequeos de sobregiro no tienen
        carrera, y el saldo se puede volver a derivar de los postings en
        cualquier momento para probarlo.
      </P>
      <P>
        <Strong>6. Conciliación continua contra el mundo exterior.</Strong> El
        estado del proveedor, el feed del banco, el reporte del procesador —
        ingeridos y diferenciados contra el ledger de forma continua, no
        trimestral. La Cicatriz 1 se descubrió por accidente; debió ser una
        alerta en menos de un día.
      </P>
      <P>
        Ninguna de estas ideas es nueva. Los contadores tienen doble entrada
        desde hace cinco siglos. Lo nuevo es la constancia con que los equipos
        de software redescubrimos, a escala de producción y con costo real,
        por qué existe cada una de estas reglas.
      </P>
      <P>
        Me cansé de reconstruir esto a partir de cicatrices, así que lo estoy
        construyendo como producto: LedgerCore, un ledger como servicio armado
        exactamente sobre estos principios.{" "}
        <a
          href="https://ledgercore.sanchezavila.com"
          className="font-medium text-accent underline decoration-accent/40 underline-offset-4 transition-colors hover:decoration-accent"
        >
          Hay un sandbox
        </a>{" "}
        si querés meterle mano. En cualquier caso: si ahora mismo estás
        guardando saldos en una columna mutable, andá a revisar cómo se
        sembraron tus saldos de apertura. Te espero.
      </P>
    </>
  );
}

function BodyEn() {
  return (
    <>
      <P>
        I found it on a Tuesday, cross-referencing a payment provider&apos;s
        settlement statement against our own books. The provider said we owed
        them six figures. Our systems said we owed them nothing. Not &ldquo;a
        smaller number&rdquo; — nothing. The debt did not exist anywhere
        inside the company.
      </P>
      <P>
        I spent years operating money-movement infrastructure for a remittance
        platform. Real volume, real regulators, real payout partners in
        multiple countries. This post is about the scars that operation left
        on me, and the design principles I refuse to compromise on now. If
        you&apos;re building anything that touches money on top of ordinary
        application tables, some of this will feel uncomfortably familiar.
      </P>

      <H2>Why operational tables + spreadsheets fail structurally</H2>
      <P>
        Most fintech-adjacent systems start the same way: an{" "}
        <Code>orders</Code>-style table, a <Code>balance</Code> column
        somewhere, and a growing constellation of spreadsheets doing
        reconciliation on the side. It works right up until it doesn&apos;t,
        and the failure is structural, not incidental:
      </P>
      <UL>
        <li>
          Operational tables are mutable. A row that says &ldquo;completed&rdquo;
          today can say something else tomorrow, and nothing records the
          transition.
        </li>
        <li>
          Balances stored as columns drift from the transactions that
          supposedly produced them, because updates to the two are not atomic.
        </li>
        <li>
          Spreadsheets are the escape hatch, and spreadsheets are hostile to
          correctness: studies have found errors in roughly 94% of
          spreadsheets in use, and around 52% of companies report material
          reconciliation exceptions in their close process.
        </li>
      </UL>
      <P>
        None of that is because people are careless. It&apos;s because the
        data model has no concept of financial truth. It has a concept of{" "}
        <em className="text-ink">current state</em>, which is a different,
        weaker thing.
      </P>
      <P>
        Here are the five scars that taught me that, each with the lesson it
        burned in.
      </P>

      <H2>Scar 1: The debt that didn&apos;t exist</H2>
      <P>
        When we finally built a proper ledger, we seeded it with an opening
        entry derived from current balances. The seeding logic had one quiet
        assumption: negative balances were noise, so it discarded them.
      </P>
      <P>
        One of those &ldquo;noise&rdquo; values was our position with a payout
        provider. We held a <em className="text-ink">negative</em> custody
        balance with them — they had paid out on our behalf beyond what we had
        funded. That is a liability. A real, six-figure debt. The opening
        entry threw it away, so the ledger was born already lying, and every
        report downstream inherited the lie.
      </P>
      <P>
        <Strong>Lesson:</Strong> a negative custody balance is a liability,
        not an anomaly. Any ledger bootstrap that filters by sign is silently
        choosing which reality to keep. Double-entry makes this class of bug
        loud: you cannot discard one side of a position without the books
        failing to balance.
      </P>

      <H2>Scar 2: The wallet that was 97% frozen</H2>
      <P>
        A business customer&apos;s wallet showed ~97% of its balance locked in
        reserves — with zero open transactions. Weeks of holds, no releases.
      </P>
      <P>
        Root cause: the reserve-release logic detected &ldquo;this order used
        the wallet&rdquo; by checking a collateral field on a payment-detail
        record. A newer transaction flow simply didn&apos;t create that
        record. Orders completed fine; the release check found nothing to
        release; reserves accumulated forever.
      </P>
      <P>
        <Strong>Lesson:</Strong> if a money-affecting decision depends on the{" "}
        <em className="text-ink">incidental shape</em> of operational data,
        every new flow is a chance to break it silently. Holds and releases
        must be first-class ledger entries — a hold is a posting, a release is
        a posting, and &ldquo;how much is reserved&rdquo; is a balance, not an
        inference over side tables.
      </P>

      <H2>Scar 3: Balances computed by summing everything, every time</H2>
      <P>
        Balance checks worked by loading all of a wallet&apos;s movement rows
        and summing them in memory. On every request. It was slow, it got
        slower linearly with history, and worse: two concurrent requests could
        both read the same &ldquo;available&rdquo; figure and both spend it.
      </P>
      <P>
        <Strong>Lesson:</Strong> materialize balances in the same database
        transaction as the posting that changes them. The posting rows remain
        the source of truth; the materialized balance is a cache that is{" "}
        <em className="text-ink">never allowed to drift</em>, because it moves
        atomically with the entries. You get O(1) reads and a natural place to
        enforce overdraft rules.
      </P>

      <H2>Scar 4: &ldquo;1.500&rdquo; became 1500</H2>
      <P>
        We had a helper that normalized user-entered decimal amounts. Given{" "}
        <Code>&quot;1.500&quot;</Code>, it decided the dot was a thousands
        separator and returned 1500. A ×1000 error in a money path.
      </P>
      <P>
        The best part: there was a unit test asserting exactly that behavior.
        The bug had been enshrined as a specification. Whoever wrote the test
        looked at the wrong output and wrote it down as correct.
      </P>
      <P>
        <Strong>Lesson:</Strong> floats and locale-sniffing string parsers
        have no place near money. Amounts should be integers in minor units,
        tagged with an asset code and an exponent (
        <Code>amount: 150000, asset: &quot;USD&quot;, exponent: 2</Code>).
        Parsing happens once, at the edge, against an explicit format — never
        by guessing what a dot means.
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
        <Strong>Lesson:</Strong> if your data model can&apos;t replay history,
        every incident becomes archaeology. Append-only isn&apos;t a purity
        preference; it&apos;s the difference between &ldquo;query the
        timeline&rdquo; and &ldquo;interview the survivors.&rdquo;
      </P>

      <H2>The principles I built into the ledger afterwards</H2>
      <P>
        These aren&apos;t aspirations. Each one is a scar with the polarity
        reversed.
      </P>
      <P>
        <Strong>1. Double-entry, enforced, not encouraged.</Strong> Every
        transaction is a set of postings that sums to zero per asset. Not by
        convention — by check, at write time:
      </P>
      <CodeBlock code={SQL_BALANCE_CHECK} />
      <P>
        If this had existed on day one, Scar 1 is impossible: dropping the
        liability side of the opening entry fails to commit.
      </P>
      <P>
        <Strong>2. Append-only, with corrections as compensating entries.</Strong>{" "}
        No UPDATE, no DELETE on posting tables — enforced in the database
        itself, so even a well-intentioned migration script can&apos;t
        rewrite history:
      </P>
      <CodeBlock code={SQL_APPEND_ONLY} />
      <P>
        Made a mistake? Post the reversal and the corrected entry. The mistake
        stays visible. That visibility is the feature.
      </P>
      <P>
        <Strong>3. Money as integers: asset + exponent.</Strong> No floats, no
        strings, no locale guessing. <Code>1500</Code> USD is{" "}
        <Code>{"{amount: 150000, asset: \"USD\", exponent: 2}"}</Code>{" "}
        everywhere below the presentation layer. Scar 4 becomes a rendering
        question, not a solvency question.
      </P>
      <P>
        <Strong>4. Idempotency end to end.</Strong> Every external command
        carries an idempotency key; replays return the original result instead
        of posting twice. Retries, webhook redeliveries, and impatient clients
        stop being money events.
      </P>
      <P>
        <Strong>5. Balances materialized in the same transaction.</Strong>{" "}
        Posting and balance update commit together or not at all. Reads are
        O(1), overdraft checks are race-free, and the balance can be
        re-derived from postings at any time to prove it.
      </P>
      <P>
        <Strong>6. Continuous reconciliation against the outside world.</Strong>{" "}
        The provider&apos;s statement, the bank feed, the processor report —
        ingested and diffed against the ledger continuously, not quarterly.
        Scar 1 was discovered by accident; it should have been an alert within
        a day.
      </P>
      <P>
        None of these ideas are new. Accountants have had double-entry for
        five centuries. What&apos;s new is how consistently software teams
        rediscover, at production scale and real cost, why every one of these
        rules exists.
      </P>
      <P>
        I got tired of rebuilding this from scars, so I&apos;m building it as
        a product: LedgerCore, a ledger-as-a-service around exactly these
        principles.{" "}
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
    </>
  );
}

export function SixFigureDebtClient() {
  return (
    <BlogPostShell slug="the-six-figure-debt">
      <PostBody es={<BodyEs />} en={<BodyEn />} />
    </BlogPostShell>
  );
}
