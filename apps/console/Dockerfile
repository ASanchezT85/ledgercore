# syntax=docker/dockerfile:1
# LedgerCore Console — multi-stage build with Next.js standalone output.
# Build context = repository ROOT (consistent with the Go services):
#   docker build -f apps/console/Dockerfile -t ledgercore/console .

FROM node:26-alpine AS base
RUN npm install -g pnpm@11
WORKDIR /app

# --- dependencies ---
FROM base AS deps
COPY apps/console/package.json apps/console/pnpm-lock.yaml apps/console/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

# --- build ---
FROM base AS build
COPY --from=deps /app/node_modules ./node_modules
COPY apps/console/ ./
# CI=true stops pnpm's deps-status check from trying to purge node_modules
# (no TTY inside docker build -> ERR_PNPM_ABORTED_REMOVE_MODULES_DIR_NO_TTY).
ENV NEXT_TELEMETRY_DISABLED=1 \
    CI=true
RUN pnpm build

# --- runtime ---
FROM node:26-alpine AS runner
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0
WORKDIR /app
RUN addgroup -S nodejs && adduser -S nextjs -G nodejs

COPY --from=build --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=build --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=build --chown=nextjs:nodejs /app/public ./public

USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
