import type { Preview } from '@storybook/react-vite';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import '../src/index.css';
import { setMockApiScenario, type MockApiScenario } from '../src/mocks/apiProxy';

const preview: Preview = {
  decorators: [
    (Story, context) => {
      const initialPath = context.parameters.initialPath ?? '/';
      const routePath = context.parameters.routePath ?? '*';
      setMockApiScenario((context.parameters.mockApiScenario ?? 'default') as MockApiScenario);
      return (
        <MemoryRouter initialEntries={[initialPath]}>
          <Routes>
            <Route path={routePath} element={<Story />} />
          </Routes>
        </MemoryRouter>
      );
    },
  ],
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    a11y: { test: 'todo' },
    backgrounds: { disable: true },
  },
};

export default preview;
