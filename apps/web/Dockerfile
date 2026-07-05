FROM node:22-alpine AS deps

WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

RUN corepack enable && corepack prepare pnpm@10.18.2 --activate

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

FROM node:22-alpine AS build

WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

ARG NEXT_PUBLIC_HIVY_SENTRY_DSN
ARG NEXT_PUBLIC_HIVY_SENTRY_TRACES_SAMPLE_RATE

ENV NEXT_PUBLIC_HIVY_SENTRY_DSN=$NEXT_PUBLIC_HIVY_SENTRY_DSN
ENV NEXT_PUBLIC_HIVY_SENTRY_TRACES_SAMPLE_RATE=$NEXT_PUBLIC_HIVY_SENTRY_TRACES_SAMPLE_RATE

RUN corepack enable && corepack prepare pnpm@10.18.2 --activate

COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN pnpm build

FROM node:22-alpine AS runner

WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=8080

RUN corepack enable && corepack prepare pnpm@10.18.2 --activate

COPY --from=build /app ./

EXPOSE 8080

CMD ["sh", "-c", "pnpm exec next start -p ${PORT:-8080} -H 0.0.0.0"]
