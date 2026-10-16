import type { Meta, StoryObj } from '@storybook/react-vite';
import { EnvironmentPage } from './EnvironmentPage';

function fetchMock(scenario: 'ready' | 'failed') {
  let polls = 0;
  return (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = (data: unknown, status = 200) =>
      new Response(JSON.stringify({ data, error: null }), {
        status,
        headers: { 'Content-Type': 'application/json' },
      });
    if (url.endsWith('/practice-environments/requests') && method === 'POST') {
      return body(
        {
          id: 'envop_story_1',
          technology: 'spring-boot',
          objective: 'practice dependency injection with services',
          status: 'pending',
          stage: 'resolving',
        },
        202,
      );
    }
    if (url.includes('/practice-environments/requests/') && method === 'GET') {
      polls += 1;
      if (scenario === 'failed' || polls < 2) {
        return body({
          id: 'envop_story_1',
          technology: 'spring-boot',
          objective: 'practice dependency injection with services',
          status: scenario === 'failed' ? 'failed' : 'pending',
          stage: scenario === 'failed' ? 'build' : 'resolving',
          error:
            scenario === 'failed'
              ? 'build stage failed: mvn -o -q test-compile exited 1 (missing spring-boot-starter-parent:3.3.5 in the dependency cache)'
              : undefined,
        });
      }
      return body({
        id: 'envop_story_1',
        technology: 'spring-boot',
        objective: 'practice dependency injection with services',
        status: 'ready',
        stage: 'ready',
        session_id: 'practice_sess_story_env',
      });
    }
    return body(null, 501);
  }) as typeof fetch;
}

const meta = {
  title: 'Environments/EnvironmentPage',
  component: EnvironmentPage,
  parameters: {
    initialPath: '/environments',
    routePath: '/environments',
  },
  decorators: [
    (Story) => {
      globalThis.fetch = fetchMock('ready');
      return <Story />;
    },
  ],
} satisfies Meta<typeof EnvironmentPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Failed: Story = {
  decorators: [
    (Story) => {
      globalThis.fetch = fetchMock('failed');
      return <Story />;
    },
  ],
};
