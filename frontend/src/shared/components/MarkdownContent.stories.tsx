import type { Meta, StoryObj } from '@storybook/react-vite';

import { MarkdownContent } from './MarkdownContent';

const meta: Meta<typeof MarkdownContent> = {
  title: 'Components/MarkdownContent',
  component: MarkdownContent,
  parameters: {
    layout: 'centered',
  },
};

export default meta;
type Story = StoryObj<typeof MarkdownContent>;

export const MarathonSurfaces: Story = {
  render: () => (
    <div className="w-[min(42rem,calc(100vw-2rem))] space-y-6 rounded-xl border bg-background p-5 text-foreground">
      <section className="space-y-2">
        <div className="text-muted-foreground text-xs font-medium">Question stem</div>
        <h2 className="text-xl leading-7 font-semibold">
          <MarkdownContent variant="title">
            {'Which call appends `item` to `items`?'}
          </MarkdownContent>
        </h2>
      </section>

      <section className="space-y-2">
        <div className="text-muted-foreground text-xs font-medium">Option labels</div>
        <div className="space-y-2">
          {['`items.push(item)`', '`items.pop()`', '<button>items.add(item)</button>'].map(
            (option) => (
              <div key={option} className="rounded-lg border px-4 py-3 text-sm">
                <MarkdownContent variant="inline">{option}</MarkdownContent>
              </div>
            ),
          )}
        </div>
      </section>

      <section className="space-y-2">
        <div className="text-muted-foreground text-xs font-medium">Help / rationale copy</div>
        <div className="rounded-lg border bg-muted/30 px-4 py-3 text-sm">
          <MarkdownContent>
            {
              'Use `Array.prototype.push` when you want to mutate the existing array. **Raw HTML** like <img src="x" onerror="alert(1)"> is displayed as text, not executed.'
            }
          </MarkdownContent>
        </div>
      </section>

      <section className="space-y-2">
        <div className="text-muted-foreground text-xs font-medium">Plain text baseline</div>
        <div className="rounded-lg border px-4 py-3 text-sm">
          <MarkdownContent variant="inline">Append to a dynamic array</MarkdownContent>
        </div>
      </section>
    </div>
  ),
};
