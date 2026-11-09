import type { Meta, StoryObj } from '@storybook/react-vite';
import { PracticePage } from './PracticePage';

const meta = {
  title: 'Practice/PracticePage',
  component: PracticePage,
  parameters: {
    initialPath: '/practice',
    routePath: '/practice',
  },
  decorators: [
    (Story) => {
      globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = (init?.method ?? 'GET').toUpperCase();
        const body = (data: unknown, status = 200) =>
          new Response(JSON.stringify({ data, error: null }), {
            status,
            headers: { 'Content-Type': 'application/json' },
          });
        if (url.endsWith('/practice/sessions') && method === 'POST') {
          return body(
            {
              session_id: 'practice_sess_story',
              owner: 'story-user',
              workspace_id: 'personal-story',
              artifact_id: 'greet-basics-2026-10-01',
              artifact_version: '0.1.0',
              technology: 'python',
              environment_base: 'python-3.13',
              network_policy: 'block-all',
              simulated: true,
              instructions: 'Implement `greet(name)` in `solution.py`.',
              editable: ['solution.py'],
              files: [{ path: 'solution.py', content: 'def greet(name):\n    raise NotImplementedError\n' }],
            },
            201,
          );
        }
        if (url.endsWith('/practice/saved') && method === 'GET') {
          return body([]);
        }
        return body(null, 501);
      }) as typeof fetch;
      return <Story />;
    },
  ],
} satisfies Meta<typeof PracticePage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
