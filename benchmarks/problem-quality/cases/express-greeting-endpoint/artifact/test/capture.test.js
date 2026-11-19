'use strict';

const assert = require('node:assert/strict');
const http = require('node:http');
const test = require('node:test');

const {
  CaptureOverflowError,
  collectResponseBody,
  createBoundedCapture,
} = require('./capture');

test('bounded capture keeps at most the ceiling and reports overflow once', () => {
  const capture = createBoundedCapture(8, 'server output');

  assert.equal(capture.append('abc'), false);
  assert.equal(capture.overflowed, false);
  assert.equal(capture.length, 3);
  assert.equal(capture.text(), 'abc');

  assert.equal(capture.append('defghij'), true);
  assert.equal(capture.append('ignored'), true);
  assert.equal(capture.overflowed, true);
  assert.equal(capture.length, 8);
  assert.equal(capture.text(), 'abcdefgh');
  assert.match(capture.describe(), /\[server output truncated after 8 bytes\]/);
});

test('bounded capture accepts an exact-fit payload without overflow', () => {
  const capture = createBoundedCapture(4, 'body');

  assert.equal(capture.append('abcd'), false);
  assert.equal(capture.overflowed, false);
  assert.equal(capture.text(), 'abcd');
});

function startServer(handler) {
  return new Promise((resolve, reject) => {
    const server = http.createServer(handler);
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => resolve(server));
  });
}

function stopServer(server) {
  return new Promise((resolve) => server.close(resolve));
}

function getBody(port, limit) {
  return new Promise((resolve, reject) => {
    const request = http.get({
      host: '127.0.0.1',
      port,
      path: '/',
      agent: false,
      timeout: 3000,
    }, (response) => {
      collectResponseBody(response, limit, 'response body').then(resolve, reject);
    });
    request.on('timeout', () => request.destroy(new Error('request timed out')));
    request.on('error', reject);
  });
}

test('response bodies under the ceiling are returned intact', async () => {
  const server = await startServer((request, response) => {
    response.writeHead(200, { 'content-type': 'text/plain' });
    response.end('ok');
  });
  try {
    const body = await getBody(server.address().port, 128);
    assert.equal(body, 'ok');
  } finally {
    await stopServer(server);
  }
});

test('oversized response bodies reject clearly and preserve cleanup', async () => {
  const server = await startServer((request, response) => {
    response.writeHead(200, { 'content-type': 'text/plain' });
    response.end('x'.repeat(4096));
  });
  try {
    const error = await getBody(server.address().port, 128).then(
      () => {
        throw new Error('oversized response body was accepted');
      },
      (failure) => failure,
    );
    assert.ok(error instanceof CaptureOverflowError);
    assert.equal(error.limit, 128);
    assert.match(error.message, /response body exceeded the 128-byte capture limit/);
  } finally {
    await stopServer(server);
  }
});
