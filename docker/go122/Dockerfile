FROM golang:1.22-alpine

RUN apk add --no-cache bash curl jq

COPY codegym-run.sh /usr/local/bin/codegym-run.sh
RUN chmod +x /usr/local/bin/codegym-run.sh

WORKDIR /app

ENTRYPOINT ["/usr/local/bin/codegym-run.sh"]
