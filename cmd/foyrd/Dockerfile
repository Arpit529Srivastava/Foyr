# The binary is cross-compiled by CI (linux/arm64); this only packages it. Migrations are embedded.
FROM gcr.io/distroless/static-debian12:nonroot
COPY foyrd /foyrd
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/foyrd"]
