import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes, useSearchParams } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { renderWithProviders } from "../../test/test-utils";
import { EnvironmentPage } from "./EnvironmentPage";

function apiResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data, error: null }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function PracticeProbe() {
  const [params] = useSearchParams();
  return <p>practice session: {params.get("session")}</p>;
}

function renderEnvironmentPage(fetchImpl: typeof fetch) {
  globalThis.fetch = fetchImpl;
  return renderWithProviders(
    <Routes>
      <Route path="/environments" element={<EnvironmentPage />} />
      <Route path="/practice" element={<PracticeProbe />} />
    </Routes>,
    { initialEntries: ["/environments"] },
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("EnvironmentPage", () => {
  it(
    "requests an environment, polls, and navigates to the ready session",
    async () => {
    const user = userEvent.setup();
    let polls = 0;
    renderEnvironmentPage((async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      if (url.endsWith("/practice-environments/requests") && method === "POST") {
        return apiResponse(
          {
            id: "envop_1",
            technology: "spring-boot",
            objective: "practice dependency injection with services",
            status: "pending",
            stage: "queued",
          },
          202,
        );
      }
      if (url.includes("/practice-environments/requests/") && method === "GET") {
        polls += 1;
        if (polls < 2) {
          return apiResponse({
            id: "envop_1",
            technology: "spring-boot",
            objective: "practice dependency injection with services",
            status: "pending",
            stage: "resolving",
          });
        }
        return apiResponse({
          id: "envop_1",
          technology: "spring-boot",
          objective: "practice dependency injection with services",
          status: "ready",
          stage: "ready",
          session_id: "practice_sess_env_1",
        });
      }
      return apiResponse(null, 501);
    }) as typeof fetch);

    await user.click(screen.getByRole("button", { name: /request environment/i }));
    await waitFor(
      () => {
        expect(screen.getByText(/practice session: practice_sess_env_1/i)).toBeInTheDocument();
      },
      { timeout: 10000 },
    );
  }, 15000);

  it("surfaces a failed build with the backend error", async () => {
    const user = userEvent.setup();
    renderEnvironmentPage((async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      if (url.endsWith("/practice-environments/requests") && method === "POST") {
        return apiResponse(
          {
            id: "envop_2",
            technology: "spring-boot",
            objective: "practice dependency injection with services",
            status: "pending",
            stage: "queued",
          },
          202,
        );
      }
      if (url.includes("/practice-environments/requests/") && method === "GET") {
        return apiResponse({
          id: "envop_2",
          technology: "spring-boot",
          objective: "practice dependency injection with services",
          status: "failed",
          stage: "build",
          error: "build stage failed: missing dependency-cache prerequisite",
        });
      }
      return apiResponse(null, 501);
    }) as typeof fetch);

    await user.click(screen.getByRole("button", { name: /request environment/i }));
    await waitFor(() => {
      expect(screen.getByText(/environment build failed/i)).toBeInTheDocument();
    });
    expect(screen.getByText("build stage failed: missing dependency-cache prerequisite")).toBeInTheDocument();
  });
});
