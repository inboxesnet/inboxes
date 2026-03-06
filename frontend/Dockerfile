FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN apk add --no-cache curl
WORKDIR /app
COPY --chown=appuser:appgroup --from=builder /app/.next/standalone ./
COPY --chown=appuser:appgroup --from=builder /app/.next/static ./.next/static
USER appuser
EXPOSE 3000
ENV HOSTNAME=0.0.0.0
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -sf http://localhost:3000/ || exit 1
CMD ["node", "server.js"]
