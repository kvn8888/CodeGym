import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, vi } from "vitest";

import { renderWithProviders } from "../../test/test-utils";
import { PracticePage } from "./PracticePage";

vi.mock("@monaco-editor/react", () => ({
  default: ({
    value,
    onChange,
    options,
  }: {
    value?: string;
    onChange?: (value: string | undefined) => void;
    options?: { readOnly?: boolean };
  }) => (
    <textarea
      aria-label="practice-editor"
      value={value ?? ""}
      onChange={(event) => onChange?.(event.target.value)}
      readOnly={options?.readOnly}
    />
  ),
}));

function apiResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data, error: null }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function apiError(code: string, message: string, status = 400) {
  return new Response(
    JSON.stringify({ data: null, error: { code, message } }),
    {
      status,
      headers: { "Content-Type": "application/json" },
    },
  );
}

const sessionView = {
  session_id: "practice_sess_1",
  owner: "user-1",
  workspace_id: "workspace-1",
  artifact_id: "greet-basics-2026-10-01",
  artifact_version: "0.1.0",
  technology: "python",
  environment_base: "python-3.13",
  network_policy: "block-all",
  simulated: true,
  instructions: "Implement greet in solution.py.",
  editable: ["solution.py"],
  files: [
    {
      path: "solution.py",
      content: "def greet(name):\n    raise NotImplementedError\n",
    },
  ],
};

const saveReceipt = {
  id: "practice_saved_1",
  artifact_id: "greet-basics-2026-10-01",
  artifact_version: "0.1.0",
  technology: "python",
  environment_base: "python-3.13",
  updated_at: new Date().toISOString(),
};

function runResult() {
  return {
    category: "test-failure",
    summary: "Tests failed.",
    output: "",
    output_truncated: false,
    steps: [
      { stage: "build", exit_code: 0 },
      { stage: "test", exit_code: 1 },
    ],
    simulated: true,
    session_id: "practice_sess_1",
    artifact_id: "greet-basics-2026-10-01",
    artifact_version: "0.1.0",
    technology: "python",
    environment_base: "python-3.13",
    network_policy: "block-all",
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

type RecordedRequest = { url: string; method: string; body: unknown };

function stubPracticeFetch(
  handler: (url: string, method: string, body: unknown) => Promise<Response>,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) : null;
      return handler(url, method, body);
    }),
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("PracticePage", () => {
  it("opens the pinned sample with safe instructions and permitted files", async () => {
    const requests: RecordedRequest[] = [];
    stubPracticeFetch(async (url, method, body) => {
      requests.push({ url, method, body });
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([]);
      throw new Error(`unexpected ${method} ${url}`);
    });

    renderWithProviders(<PracticePage />);
    await screen.findByText("Browser practice");
    expect(
      await screen.findByText("Implement greet in solution.py."),
    ).toBeInTheDocument();
    expect(screen.getByText("solution.py")).toBeInTheDocument();
    expect(
      screen.getByText("SIMULATED · DEVELOPMENT ONLY"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("practice-editor")).toHaveValue(
      "def greet(name):\n    raise NotImplementedError\n",
    );
    await waitFor(() => {
      expect(
        requests.find((r) => r.url.endsWith("/practice/sessions"))?.body,
      ).toMatchObject({
        artifact_id: "greet-basics-2026-10-01",
      });
    });
  });

  it("edits the file and sends the exact latest content to Run without marking saved", async () => {
    const requests: RecordedRequest[] = [];
    stubPracticeFetch(async (url, method, body) => {
      requests.push({ url, method, body });
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([]);
      if (url.endsWith("/files") && method === "PUT") {
        const payload = body as {
          files: Array<{ path: string; content: string }>;
        };
        return apiResponse({ ...sessionView, files: payload.files });
      }
      if (url.endsWith("/run") && method === "POST")
        return apiResponse(runResult());
      throw new Error(`unexpected ${method} ${url}`);
    });

    const user = userEvent.setup();
    renderWithProviders(<PracticePage />);
    const editor = await screen.findByLabelText("practice-editor");
    const edited = 'def greet(name):\n    return "Hello, Ada!"\n';
    fireEvent.change(editor, { target: { value: edited } });
    expect(await screen.findByText("UNSAVED")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "RUN" }));
    await waitFor(() => {
      expect(requests.some((r) => r.url.endsWith("/run"))).toBe(true);
    });
    const flushed = requests.find(
      (r) => r.url.endsWith("/files") && r.method === "PUT",
    );
    expect(flushed?.body).toEqual({
      files: [{ path: "solution.py", content: edited }],
    });
    expect(await screen.findByText("TEST FAILURE")).toBeInTheDocument();
    // A flush is not a durable save: Run must never mark SAVED.
    expect(screen.getByText("UNSAVED")).toBeInTheDocument();
    expect(screen.queryByText(/^SAVED$/)).not.toBeInTheDocument();
  });

  it("sends the exact latest content to Submit without marking saved", async () => {
    const requests: RecordedRequest[] = [];
    stubPracticeFetch(async (url, method, body) => {
      requests.push({ url, method, body });
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([]);
      if (url.endsWith("/files") && method === "PUT") {
        const payload = body as {
          files: Array<{ path: string; content: string }>;
        };
        return apiResponse({ ...sessionView, files: payload.files });
      }
      if (url.endsWith("/submit") && method === "POST")
        return apiResponse(runResult());
      throw new Error(`unexpected ${method} ${url}`);
    });

    const user = userEvent.setup();
    renderWithProviders(<PracticePage />);
    const editor = await screen.findByLabelText("practice-editor");
    const edited = 'def greet(name):\n    return "LATEST-SUBMIT-CONTENT"\n';
    fireEvent.change(editor, { target: { value: edited } });
    await user.click(screen.getByRole("button", { name: "SUBMIT" }));
    await waitFor(() => {
      expect(requests.some((r) => r.url.endsWith("/submit"))).toBe(true);
    });
    const flushed = requests.find(
      (r) => r.url.endsWith("/files") && r.method === "PUT",
    );
    expect(flushed?.body).toEqual({
      files: [{ path: "solution.py", content: edited }],
    });
    expect(await screen.findByText("TEST FAILURE")).toBeInTheDocument();
    expect(screen.getByText("UNSAVED")).toBeInTheDocument();
  });

  it("marks SAVED only on Save and keeps edits made during Save as UNSAVED", async () => {
    const requests: RecordedRequest[] = [];
    const putGate = deferred<Response>();
    let putCalls = 0;
    stubPracticeFetch(async (url, method, body) => {
      requests.push({ url, method, body });
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([]);
      if (url.endsWith("/files") && method === "PUT") {
        putCalls += 1;
        if (putCalls === 1) return putGate.promise;
        const payload = body as {
          files: Array<{ path: string; content: string }>;
        };
        return apiResponse({ ...sessionView, files: payload.files });
      }
      if (url.endsWith("/save") && method === "POST")
        return apiResponse(saveReceipt, 201);
      throw new Error(`unexpected ${method} ${url}`);
    });

    const user = userEvent.setup();
    renderWithProviders(<PracticePage />);
    const editor = await screen.findByLabelText("practice-editor");
    const beforeEdit = 'def greet(name):\n    return "BEFORE-SAVE"\n';
    fireEvent.change(editor, { target: { value: beforeEdit } });

    const savePromise = user.click(
      screen.getByRole("button", { name: "SAVE" }),
    );
    await waitFor(() => expect(screen.getByText("SAVING")).toBeInTheDocument());

    // Edit while the save flush is still in flight.
    const duringEdit = 'def greet(name):\n    return "DURING-SAVE"\n';
    fireEvent.change(editor, { target: { value: duringEdit } });

    // Release the flush carrying only the pre-edit snapshot.
    putGate.resolve(
      apiResponse({
        ...sessionView,
        files: [{ path: "solution.py", content: beforeEdit }],
      }),
    );
    await savePromise;
    await waitFor(() => {
      expect(requests.some((r) => r.url.endsWith("/save"))).toBe(true);
    });
    const flushed = requests.find(
      (r) => r.url.endsWith("/files") && r.method === "PUT",
    );
    expect(flushed?.body).toEqual({
      files: [{ path: "solution.py", content: beforeEdit }],
    });
    // The save covered only the pre-edit revision: newer edits stay UNSAVED.
    expect(await screen.findByText("UNSAVED")).toBeInTheDocument();
    expect(screen.queryByText(/^SAVED$/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("practice-editor")).toHaveValue(duringEdit);
  });

  it("serializes overlapping actions: Save during Run is ignored", async () => {
    const requests: RecordedRequest[] = [];
    const putGate = deferred<Response>();
    stubPracticeFetch(async (url, method, body) => {
      requests.push({ url, method, body });
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([]);
      if (url.endsWith("/files") && method === "PUT") return putGate.promise;
      if (url.endsWith("/run") && method === "POST")
        return apiResponse(runResult());
      if (url.endsWith("/save"))
        throw new Error("Save must not run while Run is in flight");
      throw new Error(`unexpected ${method} ${url}`);
    });

    const user = userEvent.setup();
    renderWithProviders(<PracticePage />);
    await screen.findByLabelText("practice-editor");
    const runClick = user.click(screen.getByRole("button", { name: "RUN" }));
    await waitFor(() => {
      expect(requests.some((r) => r.url.endsWith("/files"))).toBe(true);
    });
    // Overlapping Save is ignored while Run holds the operation lock.
    await user.click(screen.getByRole("button", { name: "SAVE" }));
    expect(requests.some((r) => r.url.endsWith("/save"))).toBe(false);
    putGate.resolve(apiResponse(sessionView));
    await runClick;
    await waitFor(() => {
      expect(requests.some((r) => r.url.endsWith("/run"))).toBe(true);
    });
  });

  it("saves and resumes with the exact restored content", async () => {
    const edited = 'def greet(name):\n    return "RESUME-ME"\n';
    stubPracticeFetch(async (url, method, body) => {
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([
          {
            id: "practice_saved_1",
            artifact_id: "greet-basics-2026-10-01",
            artifact_version: "0.1.0",
            technology: "python",
            environment_base: "python-3.13",
            updated_at: new Date().toISOString(),
          },
        ]);
      if (url.endsWith("/files") && method === "PUT") {
        const payload = body as {
          files: Array<{ path: string; content: string }>;
        };
        return apiResponse({ ...sessionView, files: payload.files });
      }
      if (url.endsWith("/save") && method === "POST") {
        const putBody = body;
        void putBody;
        return apiResponse(saveReceipt, 201);
      }
      if (url.endsWith("/resume") && method === "POST") {
        return apiResponse(
          {
            ...sessionView,
            session_id: "practice_sess_2",
            files: [{ path: "solution.py", content: edited }],
          },
          201,
        );
      }
      throw new Error(`unexpected ${method} ${url} ${JSON.stringify(body)}`);
    });

    const user = userEvent.setup();
    renderWithProviders(<PracticePage />);
    const editor = await screen.findByLabelText("practice-editor");
    fireEvent.change(editor, { target: { value: edited } });
    await user.click(screen.getByRole("button", { name: "SAVE" }));
    await waitFor(() => expect(screen.getByText("SAVED")).toBeInTheDocument());
    const savedSection = screen.getByText("Saved exercises")
      .parentElement as HTMLElement;
    await user.click(
      within(savedSection).getByRole("button", { name: "Resume" }),
    );
    await waitFor(() => {
      expect(screen.getByLabelText("practice-editor")).toHaveValue(edited);
    });
    expect(screen.getByText("SAVED")).toBeInTheDocument();
  });

  it("shows explicit version mismatch without substitution", async () => {
    stubPracticeFetch(async (url, method) => {
      if (url.endsWith("/practice/sessions") && method === "POST") {
        return apiError(
          "practice_artifact_mismatch",
          "The requested artifact or version does not match the verified exercise; versions are never substituted.",
          409,
        );
      }
      throw new Error(`unexpected ${method} ${url}`);
    });
    renderWithProviders(<PracticePage />);
    expect(await screen.findByText(/never substituted/)).toBeInTheDocument();
  });

  it("never renders raw hidden-test content from safe responses", async () => {
    stubPracticeFetch(async (url, method) => {
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([]);
      throw new Error(`unexpected ${method} ${url}`);
    });
    renderWithProviders(<PracticePage />);
    await screen.findByText("Browser practice");
    expect(screen.queryByText(/test_greet/)).not.toBeInTheDocument();
    expect(screen.queryByText(/SAMPLE_ASSERT/)).not.toBeInTheDocument();
  });
});

describe("PracticePage saving unavailable", () => {
  it("shows saving unavailable on Save 503 and never marks SAVED", async () => {
    stubPracticeFetch(async (url, method, body) => {
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET")
        return apiResponse([]);
      if (url.endsWith("/files") && method === "PUT") {
        const payload = body as {
          files: Array<{ path: string; content: string }>;
        };
        return apiResponse({ ...sessionView, files: payload.files });
      }
      if (url.endsWith("/save") && method === "POST") {
        return apiError(
          "practice_saved_unavailable",
          "Saved exercise storage is not configured.",
          503,
        );
      }
      throw new Error(`unexpected ${method} ${url}`);
    });

    const user = userEvent.setup();
    renderWithProviders(<PracticePage />);
    await screen.findByText("Browser practice");
    await user.click(screen.getByRole("button", { name: "SAVE" }));
    expect(
      await screen.findByText(/Saving is unavailable on this server/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "SAVE" })).toBeDisabled();
    expect(screen.queryByText(/^SAVED$/)).not.toBeInTheDocument();
    expect(screen.getByText("NOT SAVED")).toBeInTheDocument();
  });

  it("shows saving unavailable when the saved list is unavailable on load", async () => {
    stubPracticeFetch(async (url, method) => {
      if (url.endsWith("/practice/sessions") && method === "POST")
        return apiResponse(sessionView, 201);
      if (url.endsWith("/practice/saved") && method === "GET") {
        return apiError(
          "practice_saved_unavailable",
          "Saved exercise storage is not configured.",
          503,
        );
      }
      throw new Error(`unexpected ${method} ${url}`);
    });

    renderWithProviders(<PracticePage />);
    await screen.findByText("Browser practice");
    expect(
      await screen.findByText(/Saving is unavailable on this server/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "SAVE" })).toBeDisabled();
  });
});
