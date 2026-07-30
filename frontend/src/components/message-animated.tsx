import * as React from 'react';
import { motion, useReducedMotion } from 'motion/react';

import { Bubble, BubbleContent } from '@/components/ui/bubble';
import { Message, MessageContent } from '@/components/ui/message';
import { MessageScrollerItem } from '@/components/ui/message-scroller';
import {
  MESSAGE_ANIMATIONS,
  type MessageAnimationPreset,
} from '@/lib/message-animations';
import { cn } from '@/lib/utils';
import { MarkdownContent } from '@/shared/components/MarkdownContent';

type MessageAnimatedPart = {
  type: string;
  text?: string;
};

export type MessageAnimatedMessage = {
  id: string;
  role: string;
  text?: string;
  parts?: ReadonlyArray<MessageAnimatedPart>;
};

type MessageAnimatedTextPart = {
  key: string;
  text: string;
};

const MotionMessageScrollerItem = motion.create(MessageScrollerItem);

function MessageAnimated({
  message,
  animationPreset = MESSAGE_ANIMATIONS['slide-up'],
  assistantVariant = 'ghost',
  scrollAnchor,
  userVariant = 'muted',
  ...props
}: Omit<
  React.ComponentProps<typeof MotionMessageScrollerItem>,
  'animate' | 'children' | 'exit' | 'initial' | 'messageId' | 'variants'
> & {
  animationPreset?: MessageAnimationPreset;
  assistantVariant?: React.ComponentProps<typeof Bubble>['variant'];
  message: MessageAnimatedMessage;
  userVariant?: React.ComponentProps<typeof Bubble>['variant'];
}) {
  const shouldReduceMotion = useReducedMotion();
  const isUserMessage = message.role === 'user';

  if (isUserMessage) {
    return (
      <MotionMessageScrollerItem
        messageId={message.id}
        scrollAnchor={scrollAnchor ?? true}
        variants={animationPreset.variants}
        initial={shouldReduceMotion ? false : 'initial'}
        animate="animate"
        exit={shouldReduceMotion ? undefined : 'exit'}
        {...props}
      >
        <MessageAnimatedRow
          message={message}
          assistantVariant={assistantVariant}
          userVariant={userVariant}
        />
      </MotionMessageScrollerItem>
    );
  }

  return (
    <MotionMessageScrollerItem
      messageId={message.id}
      scrollAnchor={scrollAnchor}
      initial={false}
      {...props}
    >
      <MessageAnimatedRow
        message={message}
        assistantVariant={assistantVariant}
        userVariant={userVariant}
      />
    </MotionMessageScrollerItem>
  );
}

function MessageAnimatedRow({
  message,
  assistantVariant,
  userVariant,
}: {
  assistantVariant: React.ComponentProps<typeof Bubble>['variant'];
  message: MessageAnimatedMessage;
  userVariant: React.ComponentProps<typeof Bubble>['variant'];
}) {
  const isUserMessage = message.role === 'user';
  const textParts = getMessageAnimatedTextParts(message);

  return (
    <Message align={isUserMessage ? 'end' : 'start'}>
      <MessageContent>
        {textParts.map((part) => (
          <Bubble
            key={part.key}
            variant={isUserMessage ? userVariant : assistantVariant}
          >
            <BubbleContent
              className={cn(
                isUserMessage ? 'whitespace-pre-wrap' : 'max-w-none space-y-0',
              )}
            >
              {isUserMessage ? (
                <p className="whitespace-pre-wrap">{part.text}</p>
              ) : (
                <MarkdownContent className="text-sm leading-relaxed">
                  {part.text}
                </MarkdownContent>
              )}
            </BubbleContent>
          </Bubble>
        ))}
      </MessageContent>
    </Message>
  );
}

function getMessageAnimatedTextParts(
  message: MessageAnimatedMessage,
): MessageAnimatedTextPart[] {
  if (message.parts) {
    return message.parts.flatMap((part, index) => {
      if (part.type !== 'text' || typeof part.text !== 'string') {
        return [];
      }

      return [{ key: `${message.id}-${index}`, text: part.text }];
    });
  }

  return typeof message.text === 'string'
    ? [{ key: `${message.id}-text`, text: message.text }]
    : [];
}

export { MessageAnimated };