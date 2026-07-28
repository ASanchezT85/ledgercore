# Rotación de secretos de webhook (con período de gracia)

Cada suscripción de webhook tiene un secreto de firma (`whsec_…`) con el que
LedgerCore firma cada entrega:

```
X-LedgerCore-Signature: t=<unix seconds>,v1=<hmac-sha256 hex>[,v1=<hex>]
```

El MAC se calcula sobre `"<t>." + cuerpo crudo` con HMAC-SHA256. El receptor
debe aceptar la cabecera si **cualquiera** de las entradas `v1` coincide con
el MAC calculado con SU secreto (comparación en tiempo constante) y rechazar
timestamps fuera de su ventana de tolerancia (recomendado: 5 minutos).

## Cómo rotar sin perder eventos

1. **Pide la rotación**:

   ```
   POST /v1/webhook-subscriptions/{id}/rotate-secret
   ```

   Respuesta (el secreto nuevo se muestra UNA sola vez):

   ```json
   {
     "id": "…",
     "secret": "whsec_NUEVO…",
     "previous_secret_expires_at": "2026-07-25T12:00:00Z"
   }
   ```

2. **Ventana de gracia (24 h)**: desde ese momento y hasta
   `previous_secret_expires_at`, cada entrega se firma con AMBOS secretos —
   la cabecera lleva dos entradas `v1` (primero la del secreto nuevo, luego
   la del anterior). Tu endpoint sigue verificando con el secreto viejo sin
   perder ni rechazar ningún evento.

3. **Actualiza tu endpoint** en cualquier momento dentro de la ventana:
   cambia el secreto configurado en tu verificador por el nuevo. Como ambas
   firmas viajan juntas, el cambio es atómico desde tu lado: antes del cambio
   verifica la entrada vieja, después la nueva.

4. **Expiración y purga**: pasadas las 24 h el secreto anterior deja de
   usarse para firmar y un barrido automático (cada hora) lo borra de la base
   de datos (`previous_secret = NULL`). Un receptor que siga anclado al
   secreto viejo dejará de verificar: rota siempre dentro de la ventana.

Rotar de nuevo durante una gracia activa reemplaza al "secreto anterior" por
el que era vigente hasta ese momento y reinicia la ventana de 24 h; el
penúltimo secreto queda invalidado de inmediato.

## Ejemplo de verificación (pseudocódigo)

```python
def verify(secret, header, body, tolerance=300):
    parts = dict/multidict of "k=v" split by ","
    t = int(parts["t"])              # rechaza si |now - t| > tolerance
    expected = hmac_sha256(secret, f"{t}.".encode() + body)
    return any(constant_time_eq(unhex(v), expected) for v in parts.getall("v1"))
```

La implementación de referencia (usada por el dispatcher y pensada para los
SDKs) vive en `services/webhooks/internal/signature`.

## Detalles operativos

- Columnas: `subscriptions.previous_secret`,
  `subscriptions.previous_secret_expires_at`
  (migración `0002_secret_rotation.sql`).
- Duración de la gracia: `domain.RotationGrace` = 24 h.
- Purga: goroutine horaria en `cmd/webhooks/main.go`
  (`PurgeExpiredPreviousSecrets`), más el filtro por expiración al firmar
  (aunque la fila siga presente, un secreto vencido jamás firma).
