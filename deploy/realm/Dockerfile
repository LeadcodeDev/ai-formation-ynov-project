FROM alpine:3.22
RUN apk add --no-cache curl jq
COPY provision.sh /provision.sh
USER nobody
ENTRYPOINT ["/bin/sh", "/provision.sh"]
