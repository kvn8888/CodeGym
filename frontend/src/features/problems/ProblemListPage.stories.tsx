import type { Meta, StoryObj } from '@storybook/react-vite';
import { ProblemListPage } from './ProblemListPage';
import type { ProblemSummary } from '../../shared/api/types';

const meta: Meta<typeof ProblemListPage> = {
  title: 'Pages/ProblemList',
  component: ProblemListPage,
  parameters: {
    initialPath: '/',
    routePath: '/',
  },
};

export default meta;
type Story = StoryObj<typeof ProblemListPage>;

const sampleProblems: ProblemSummary[] = [
  {
    id: 'two-sum',
    title: 'Two Sum',
    category: 'dsa',
    language: 'go',
    difficulty: 1,
    tags: ['arrays', 'hash-map'],
    estimated_minutes: 15,
    type: 'function',
  },
  {
    id: 'express-pagination',
    title: 'Implement Cursor-Based Pagination in Express',
    category: 'api-patterns',
    language: 'javascript',
    framework: 'express',
    difficulty: 3,
    tags: ['rest', 'pagination', 'cursor'],
    estimated_minutes: 25,
    type: 'api-server',
  },
  {
    id: 'go-fan-out',
    title: 'Fan-Out / Fan-In with Goroutines',
    category: 'concurrency',
    language: 'go',
    difficulty: 4,
    tags: ['goroutines', 'channels', 'concurrency'],
    estimated_minutes: 35,
    type: 'system',
  },
  {
    id: 'pytorch-mnist',
    title: 'Train a Simple MNIST Classifier',
    category: 'ml',
    language: 'python',
    framework: 'pytorch',
    difficulty: 3,
    tags: ['pytorch', 'neural-network', 'mnist'],
    estimated_minutes: 40,
    type: 'library-usage',
  },
  {
    id: 'linked-list-cpp',
    title: 'Implement a Singly Linked List in C++',
    category: 'dsa',
    language: 'cpp',
    difficulty: 2,
    tags: ['linked-list', 'pointers', 'data-structures'],
    estimated_minutes: 20,
    type: 'function',
  },
  {
    id: 'spring-rest',
    title: 'Build a REST API with Spring Boot',
    category: 'api-patterns',
    language: 'java',
    framework: 'spring-boot',
    difficulty: 3,
    tags: ['rest', 'spring', 'jpa'],
    estimated_minutes: 45,
    type: 'api-server',
  },
];

function makeFetchMock(problems: ProblemSummary[]) {
  return (url: string) => {
    void url;
    return Promise.resolve(
      new Response(
        JSON.stringify({ data: { problems, total: problems.length }, error: null }),
        { headers: { 'Content-Type': 'application/json' } },
      ),
    );
  };
}

export const ManyProblems: Story = {
  name: 'With Problems',
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetchMock(sampleProblems) as typeof fetch;
      return <Story />;
    },
  ],
};

export const Empty: Story = {
  name: 'No Problems Yet',
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetchMock([]) as typeof fetch;
      return <Story />;
    },
  ],
};

export const GoOnly: Story = {
  name: 'Go Problems Only',
  decorators: [
    (Story) => {
      globalThis.fetch = makeFetchMock(sampleProblems.filter((p) => p.language === 'go')) as typeof fetch;
      return <Story />;
    },
  ],
};

export const Loading: Story = {
  name: 'Loading State',
  decorators: [
    (Story) => {
      globalThis.fetch = (() => new Promise(() => {})) as typeof fetch;
      return <Story />;
    },
  ],
};
