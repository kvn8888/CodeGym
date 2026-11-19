const { spawn } = require('node:child_process');
const http = require('node:http');
const net = require('node:net');
const path = require('node:path');

const {
  CaptureOverflowError,
  DEFAULT_BODY_LIMIT_BYTES,
  DEFAULT_OUTPUT_LIMIT_BYTES,
  collectResponseBody,
  createBoundedCapture,
} = require('./capture');

const ROOT = path.resolve(__dirname, '..');
const APP = path.join(ROOT, 'src', 'app.js');
const PROTOCOL = 'CODEGYM_TEST';
const STARTUP_TIMEOUT_MS = 10000;
const REQUEST_TIMEOUT_MS = 3000;
const BODY_LIMIT_BYTES = DEFAULT_BODY_LIMIT_BYTES;
const OUTPUT_LIMIT_BYTES = DEFAULT_OUTPUT_LIMIT_BYTES;

let failures = 0;

function sanitize(value) {
  return String(value === undefined || value === null ? '(missing)' : value)
    .replace(/\s+/g, ' ')
    .trim();
}

function protocolValue(value) {
  return sanitize(value).replace(/\s+/g, '_');
}

function emit(testId, passed, code, expected, actual) {
  if (passed) {
    console.log(`${PROTOCOL} ${testId} PASS`);
    return;
  }
  failures += 1;
  console.log(`${PROTOCOL} ${testId} FAIL code=${code} expected=${protocolValue(expected)} actual=${protocolValue(actual)}`);
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.unref();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      server.close(() => resolve(address.port));
    });
  });
}

function request(port, requestPath) {
  return new Promise((resolve, reject) => {
    const req = http.get({
      host: '127.0.0.1',
      port,
      path: requestPath,
      agent: false,
      timeout: REQUEST_TIMEOUT_MS,
    }, (res) => {
      collectResponseBody(res, BODY_LIMIT_BYTES, 'response body')
        .then((body) => {
          let json = null;
          try {
            json = JSON.parse(body);
          } catch {
            json = null;
          }
          resolve({
            status: res.statusCode,
            contentType: String(res.headers['content-type'] || ''),
            body,
            json,
          });
        })
        .catch(reject);
    });
    req.on('timeout', () => req.destroy(new Error('request timed out')));
    req.on('error', reject);
  });
}

function greetingPath(name) {
  return `/greeting?${new URLSearchParams({ name }).toString()}`;
}

async function waitForReady(port, child) {
  const deadline = Date.now() + STARTUP_TIMEOUT_MS;
  let lastError = 'no response';
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`server exited before readiness with code ${child.exitCode}`);
    }
    try {
      await request(port, '/greeting?name=Ready');
      return;
    } catch (error) {
      lastError = error && error.message ? error.message : String(error);
    }
    await delay(100);
  }
  throw new Error(`server did not become ready: ${lastError}`);
}

async function stopServer(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) {
    return;
  }
  const exited = new Promise((resolve) => child.once('exit', resolve));
  child.kill('SIGTERM');
  const killer = setTimeout(() => {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill('SIGKILL');
    }
  }, 2000);
  killer.unref();
  await Promise.race([exited, delay(3000)]);
  clearTimeout(killer);
}

async function runCase(testId, fn) {
  try {
    await fn();
  } catch (error) {
    const code = error instanceof CaptureOverflowError ? 'RESPONSE_OVERFLOW' : 'REQUEST_ERROR';
    emit(testId, false, code, 'response', error && error.message ? error.message : error);
  }
}

async function main() {
  let child;
  const stdout = createBoundedCapture(OUTPUT_LIMIT_BYTES, 'server stdout');
  const stderr = createBoundedCapture(OUTPUT_LIMIT_BYTES, 'server stderr');
  try {
    const port = await getFreePort();
    child = spawn(process.execPath, [APP], {
      cwd: ROOT,
      env: { ...process.env, PORT: String(port), NODE_ENV: 'test' },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    child.stdout.on('data', (chunk) => {
      stdout.append(chunk);
    });
    child.stderr.on('data', (chunk) => {
      stderr.append(chunk);
    });

    await waitForReady(port, child);

    await runCase('hidden-success', async () => {
      const res = await request(port, greetingPath('Ada'));
      if (res.status !== 200) {
        return emit('hidden-success', false, 'HTTP_STATUS', '200', res.status);
      }
      if (!res.contentType.toLowerCase().includes('application/json')) {
        return emit('hidden-success', false, 'CONTENT_TYPE', 'application/json', res.contentType);
      }
      return emit('hidden-success', true);
    });

    await runCase('hidden-response-shape', async () => {
      const res = await request(port, greetingPath('Ada'));
      if (res.status !== 200) {
        return emit('hidden-response-shape', false, 'HTTP_STATUS', '200', res.status);
      }
      const keys = res.json && typeof res.json === 'object' ? Object.keys(res.json) : [];
      if (!keys.includes('message')) {
        return emit('hidden-response-shape', false, 'JSON_FIELD', 'message', keys.length ? keys[0] : '(missing)');
      }
      if (res.json.message !== 'Hello, Ada!') {
        return emit('hidden-response-shape', false, 'JSON_VALUE', 'Hello, Ada!', res.json.message);
      }
      return emit('hidden-response-shape', true);
    });

    await runCase('hidden-missing-name', async () => {
      const res = await request(port, '/greeting');
      if (res.status !== 400) {
        return emit('hidden-missing-name', false, 'HTTP_STATUS', '400', res.status);
      }
      if (!res.contentType.toLowerCase().includes('application/json')) {
        return emit('hidden-missing-name', false, 'CONTENT_TYPE', 'application/json', res.contentType);
      }
      if (!res.json || res.json.error !== 'name is required') {
        return emit('hidden-missing-name', false, 'JSON_VALUE', 'name is required', res.json && res.json.error);
      }
      return emit('hidden-missing-name', true);
    });

    await runCase('hidden-blank-name', async () => {
      const res = await request(port, greetingPath('   '));
      if (res.status !== 400) {
        return emit('hidden-blank-name', false, 'HTTP_STATUS', '400', res.status);
      }
      if (!res.json || res.json.error !== 'name is required') {
        return emit('hidden-blank-name', false, 'JSON_VALUE', 'name is required', res.json && res.json.error);
      }
      return emit('hidden-blank-name', true);
    });

    await runCase('hidden-unknown-query', async () => {
      const res = await request(port, '/greeting?name=Ada&extra=ignored');
      if (res.status !== 200) {
        return emit('hidden-unknown-query', false, 'HTTP_STATUS', '200', res.status);
      }
      if (!res.json || res.json.message !== 'Hello, Ada!') {
        return emit('hidden-unknown-query', false, 'JSON_VALUE', 'Hello, Ada!', res.json && res.json.message);
      }
      return emit('hidden-unknown-query', true);
    });

    await runCase('hidden-no-trimming', async () => {
      const res = await request(port, greetingPath(' Ada '));
      if (res.status !== 200) {
        return emit('hidden-no-trimming', false, 'HTTP_STATUS', '200', res.status);
      }
      if (!res.json || res.json.message !== 'Hello,  Ada !') {
        return emit('hidden-no-trimming', false, 'JSON_VALUE', 'Hello,  Ada !', res.json && res.json.message);
      }
      return emit('hidden-no-trimming', true);
    });
  } catch (error) {
    emit('hidden-server-startup', false, 'SERVER_STARTUP', 'ready', error && error.message ? error.message : error);
    if (stdout.length > 0) {
      process.stderr.write(`--- server stdout ---\n${stdout.describe()}\n`);
    }
    if (stderr.length > 0) {
      process.stderr.write(`--- server stderr ---\n${stderr.describe()}\n`);
    }
  } finally {
    await stopServer(child);
  }
  if (stdout.overflowed || stderr.overflowed) {
    emit(
      'hidden-server-output',
      false,
      'OUTPUT_OVERFLOW',
      `at_most_${OUTPUT_LIMIT_BYTES}_bytes`,
      `${stdout.length + stderr.length}_captured_bytes`,
    );
  }
  if (failures > 0) {
    process.exitCode = 1;
  }
}

main().catch((error) => {
  emit('hidden-server-startup', false, 'SERVER_STARTUP', 'ready', error && error.message ? error.message : error);
  process.exitCode = 1;
});
