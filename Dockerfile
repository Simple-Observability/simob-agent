ARG TARGETARCH
FROM alpine:3.19

RUN addgroup -S simob && adduser -S simob -G simob
WORKDIR /app
COPY simob-linux-${TARGETARCH} /app/simob
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && chown -R simob:simob /app
USER simob
ENTRYPOINT ["/app/entrypoint.sh"]
