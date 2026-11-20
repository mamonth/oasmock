FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app

COPY --chmod=+x oasmock-${TARGETOS}-${TARGETARCH}* /app/oasmock

EXPOSE 19191 8443

ENTRYPOINT ["/app/oasmock"]
