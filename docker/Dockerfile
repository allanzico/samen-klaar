FROM node:22-alpine AS frontend
WORKDIR /workspace
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html svelte.config.js tsconfig.json vite.config.ts ./
COPY src ./src
COPY tests/browser ./tests/browser
COPY public ./public
RUN npm run test:controller && npm run test:names && npm run test:format && npm run test:interaction && npm run test:i18n && npm run test:payouts && npm run check && npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY db ./db
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go test ./... && go vet ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM backend AS verification
COPY --from=frontend /workspace/dist /app/dist

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/api /api
COPY --from=frontend /workspace/dist /app/dist
ENV APP_ENV=production \
    APP_ADDR=0.0.0.0:8080 \
    WEB_DIR=/app/dist
EXPOSE 8080
ENTRYPOINT ["/api"]
