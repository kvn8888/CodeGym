FROM python:3.12-alpine

RUN apk add --no-cache bash curl jq

# Pre-install common test dependencies
RUN pip install --no-cache-dir pytest pytest-json-report

COPY codegym-run.sh /usr/local/bin/codegym-run.sh
RUN chmod +x /usr/local/bin/codegym-run.sh

WORKDIR /app

ENTRYPOINT ["/usr/local/bin/codegym-run.sh"]
