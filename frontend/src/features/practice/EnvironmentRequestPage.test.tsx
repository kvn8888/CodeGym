import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes, useLocation } from "react-router-dom";
import { afterEach, vi } from "vitest";

import { renderWithProviders } from "../../test/test-utils";
import { EnvironmentRequestPage } from "./EnvironmentRequestPage";

function apiResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data, error: null }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function PracticeLocation() {
  const location = useLocation();
  return <p>practice at {location.pathname + location.search}</p>;
}

function renderPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/practice/new" element={<EnvironmentRequestPage />} />
      <Route path="/practice" element={<PracticeLocation />} />
    </Routes>,
    { initialEntries: ["/practice/new"] },
  );
}

const base = {
  id: "envreq_1",
  technology: "Spring Boot",
  objective: "practice dependency injection with services",
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("EnvironmentRequestPage", () => {
  it("requests an environment, shows progress, and opens the practice session", async () => {
    const statuses = [
      { ...base, status: "pending", stage: "agent building project and resolving dependencies" },
      { ...base, status: "ready", stage: "opening practice session", session_id: "practice_sess_9" },
    ];
    const posted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/practice-environments/requests") && init?.method === "POST") {
          posted.push(JSON.parse(String(init.body)));
          return apiResponse({ ...base, status: "pending", stage: "queued" }, 202);
        }
        if (url.endsWith("/practice-environments/requests/envreq_1")) {
          return apiResponse(statuses.shift() ?? statuses[statuses.length - 1]);
        }
        throw new Error(`unexpected ${url}`);
      }),
    );

    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "BUILD EXERCISE" }));
    expect(posted).toEqual([
      { technology: "Spring Boot", objective: "practice dependency injection with services" },
    ]);
    expect(
      await screen.findByText("agent building project and resolving dependencies", {}, { timeout: 4000 }),
    ).toBeInTheDocument();
    await waitFor(
      () => expect(screen.getByText("practice at /practice?session=practice_sess_9")).toBeInTheDocument(),
      { timeout: 6000 },
    );
  });

  it("shows the failed stage and error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (init?.method === "POST") return apiResponse({ ...base, status: "pending", stage: "queued" }, 202);
        if (url.endsWith("/practice-environments/requests/envreq_1"))
          return apiResponse({
            ...base,
            status: "failed",
            stage: "verifying build, tests, and starter",
            error: "environment not ready: verification-failed",
          });
        throw new Error(`unexpected ${url}`);
      }),
    );

    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "BUILD EXERCISE" }));
    expect(
      await screen.findByText("environment not ready: verification-failed", {}, { timeout: 4000 }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "BUILD EXERCISE" })).toBeEnabled();
  });
});
