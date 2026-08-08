FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app

COPY oasmock-${TARGETOS}-${TARGETARCH}* /app/oasmock

EXPOSE 19191

ENTRYPOINT ["/app/oasmock"]
