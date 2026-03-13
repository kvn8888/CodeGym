FROM node:20-alpine

RUN apk add --no-cache bash curl jq

# Pre-install common Express dependencies
RUN mkdir -p /opt/codegym/node_modules && \
    cd /opt/codegym && \
    npm init -y && \
    npm install express jest supertest @types/express @types/jest @types/supertest

COPY codegym-run.sh /usr/local/bin/codegym-run.sh
RUN chmod +x /usr/local/bin/codegym-run.sh

ENV NODE_PATH=/opt/codegym/node_modules

WORKDIR /app

ENTRYPOINT ["/usr/local/bin/codegym-run.sh"]
