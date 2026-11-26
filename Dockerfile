FROM --platform=${BUILDPLATFORM} golang AS cssbuilder
WORKDIR /src

ARG BUILDARCH
# The URL uses x64 instead of amd64
RUN ARCH=$( [ "${BUILDARCH}" = "amd64" ] && echo "x64" || echo "arm64" ) && \
  curl -sfL -o tailwindcss https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-${ARCH}
RUN chmod a+x tailwindcss

COPY tailwind.css ./

COPY go.mod go.sum ./
RUN go mod download

COPY html ./html/

RUN ./tailwindcss -i tailwind.css -o app.css --minify

# No page may ask another host for anything. So the build fails if the stylesheet, comments aside,
# imports anything, has a url() that is not one of the app's own fonts, or names a scheme or a host at all.
RUN sed -E 's@/\*([^*]|\*+[^*/])*\*+/@@g' app.css > app.bare.css \
  && ! grep -Eio '@import[^;]*|url\([^)]*\)' app.bare.css | grep -Ev '^url\("?fonts/[A-Za-z0-9-]+\.(ttf|woff2)"?\)$' \
  && ! grep -Eo '.{0,60}//.{0,60}' app.bare.css



FROM golang AS gobuilder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

ARG TARGETARCH
RUN GOOS=linux GOARCH=${TARGETARCH} CGO_ENABLED=1 go build -tags sqlite_fts5,sqlite_math_functions -o /bin/app ./cmd/app



FROM debian:trixie-slim AS runner
WORKDIR /app

RUN set -x && apt-get update && \
  DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates && \
  rm -rf /var/lib/apt/lists/*

COPY public ./public/
COPY sqlite/migrations ./sqlite/migrations/
COPY --from=cssbuilder /src/app.css ./public/styles/
COPY --from=gobuilder /bin/app ./

EXPOSE 8080

CMD ["./app"]
