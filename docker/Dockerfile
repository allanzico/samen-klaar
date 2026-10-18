FROM node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402 AS frontend
WORKDIR /workspace
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html svelte.config.js tsconfig.json vite.config.ts ./
COPY src ./src
COPY public ./public
RUN npm run test:controller && npm run check && npm run build

FROM golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS backend
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY db ./db
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go test ./... && go vet ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM backend AS verification
COPY --from=frontend /workspace/dist /app/dist

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=backend /out/api /api
COPY --from=frontend /workspace/dist /app/dist
ENV APP_ENV=production \
    APP_ADDR=0.0.0.0:8080 \
    WEB_DIR=/app/dist
EXPOSE 8080
ENTRYPOINT ["/api"]
