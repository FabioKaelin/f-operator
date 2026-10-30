FROM envoyproxy/envoy:distroless-v1.39.6@sha256:1cac8b3a2f72d5a0918f54d7b49e68aae0b0ba325c2d50c8a9a3fff7a4d1f994
COPY --chmod=0755 image/envoy /usr/local/bin/envoy
LABEL org.opencontainers.image.source="https://github.com/FabioKaelin/f-operator" \
      org.opencontainers.image.version="1.39.6-gperftools" \
      org.opencontainers.image.revision="aacd0340210b536e99ec44ebe59ab9d16b259552"
