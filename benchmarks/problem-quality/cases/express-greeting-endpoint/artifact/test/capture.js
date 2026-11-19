'use strict';

const DEFAULT_OUTPUT_LIMIT_BYTES = 64 * 1024;
const DEFAULT_BODY_LIMIT_BYTES = 256 * 1024;

class CaptureOverflowError extends Error {
  constructor(label, limit) {
    super(`${label} exceeded the ${limit}-byte capture limit`);
    this.name = 'CaptureOverflowError';
    this.label = label;
    this.limit = limit;
  }
}

function createBoundedCapture(limit, label) {
  if (!Number.isInteger(limit) || limit < 0) {
    throw new TypeError('capture limit must be a non-negative integer');
  }

  const chunks = [];
  const name = label || 'output';
  let captured = 0;
  let overflowed = false;

  return {
    append(chunk) {
      const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk));
      const remaining = limit - captured;
      if (buffer.length <= remaining) {
        if (buffer.length > 0) {
          chunks.push(buffer);
          captured += buffer.length;
        }
        return false;
      }
      if (remaining > 0) {
        chunks.push(buffer.subarray(0, remaining));
        captured = limit;
      }
      overflowed = true;
      return true;
    },
    get overflowed() {
      return overflowed;
    },
    get length() {
      return captured;
    },
    get limit() {
      return limit;
    },
    text() {
      return Buffer.concat(chunks).toString('utf8');
    },
    describe() {
      const text = this.text();
      if (!overflowed) {
        return text;
      }
      return `${text}\n...[${name} truncated after ${limit} bytes]`;
    },
  };
}

function collectResponseBody(response, limit, label) {
  return new Promise((resolve, reject) => {
    const capture = createBoundedCapture(limit, label);
    let settled = false;

    const fail = (error) => {
      if (settled) {
        return;
      }
      settled = true;
      response.destroy();
      reject(error);
    };

    response.on('data', (chunk) => {
      if (capture.append(chunk)) {
        fail(new CaptureOverflowError(label || 'response body', limit));
      }
    });
    response.on('end', () => {
      if (settled) {
        return;
      }
      settled = true;
      resolve(capture.text());
    });
    response.on('error', (error) => {
      if (settled) {
        return;
      }
      settled = true;
      reject(error);
    });
  });
}

module.exports = {
  DEFAULT_BODY_LIMIT_BYTES,
  DEFAULT_OUTPUT_LIMIT_BYTES,
  CaptureOverflowError,
  collectResponseBody,
  createBoundedCapture,
};
