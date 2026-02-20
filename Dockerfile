# TEMPORARY WORKAROUND: The binary is built on the host and copied in rather
# than compiled inside a multi-stage Docker build. This is because go.mod has a
# local replace directive (../aks-unbounded/stretch) that cannot be resolved
# inside the container build context. Once that dependency is published to a
# registry, this Dockerfile should be replaced with a proper multi-stage build.
FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH
ARG BINARY_NAME=controller

COPY bin/${BINARY_NAME}-linux-${TARGETARCH} /controller

ENTRYPOINT ["/controller"]
