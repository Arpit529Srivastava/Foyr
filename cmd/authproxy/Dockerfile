# The binary is cross-compiled by CI (linux/arm64); this only packages it.
FROM gcr.io/distroless/static-debian12:nonroot
COPY authproxy /authproxy
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/authproxy"]
