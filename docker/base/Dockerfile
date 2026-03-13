FROM alpine:3.21

RUN apk add --no-cache bash curl jq

# Common entrypoint script
COPY codegym-run.sh /usr/local/bin/codegym-run.sh
RUN chmod +x /usr/local/bin/codegym-run.sh

WORKDIR /app
