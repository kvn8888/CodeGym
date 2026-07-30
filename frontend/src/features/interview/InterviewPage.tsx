import { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, CheckCircle2, MessageSquareText, RefreshCw } from 'lucide-react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ChatPanel } from '../chat/ChatPanel';
import { api } from '../../shared/api/client';
import type {
  ChatMessage,
  ChatThread,
  ChatThreadWithMessages,
  InterviewAssessment,
  InterviewFinishResult,
  InterviewMode,
  PracticeSession,
} from '../../shared/api/types';
import { GridSpinner } from '../../shared/components/GridSpinner';
import {
  WorkspaceEmptyState,
  WorkspacePage,
  WorkspacePageHeader,
} from '../../shared/components/WorkspacePage';

function sessionMode(session: PracticeSession): InterviewMode {
  const state = session.state && typeof session.state === 'object'
    ? (session.state as Record<string, unknown>)
    : {};
  const mode = state.mode;
  if (mode === 'coding' || mode === 'system_design' || mode === 'behavioral' || mode === 'open_coaching') {
    return mode;
  }
  return 'open_coaching';
}

function modeLabel(mode: InterviewMode) {
  if (mode === 'system_design') return 'System design';
  if (mode === 'open_coaching') return 'Open coaching';
  return mode.charAt(0).toUpperCase() + mode.slice(1);
}

export function InterviewPage() {
  const { sessionId = '' } = useParams();
  const navigate = useNavigate();
  const [session, setSession] = useState<PracticeSession | null>(null);
  const [completedThread, setCompletedThread] = useState<ChatThreadWithMessages | null>(null);
  const [activeThread, setActiveThread] = useState<ChatThreadWithMessages | null>(null);
  const [assessment, setAssessment] = useState<InterviewAssessment | null>(null);
  const [memoryStatus, setMemoryStatus] = useState<'synced' | 'failed' | null>(null);
  const [loading, setLoading] = useState(true);
  const [finishing, setFinishing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void api.get<PracticeSession>(`/sessions/${encodeURIComponent(sessionId)}`)
      .then(async (nextSession) => {
        if (cancelled) return;
        setSession(nextSession);
        if (nextSession.status === 'completed') {
          const list = await api.get<{ threads: ChatThread[] }>(
            `/chat/threads?kind=interview&status=completed&session_id=${encodeURIComponent(sessionId)}&limit=1`,
          );
          const thread = list.threads[0];
          if (thread) {
            const history = await api.get<{ messages: ChatMessage[] }>(
              `/chat/threads/${encodeURIComponent(thread.id)}/messages`,
            );
            if (!cancelled) setCompletedThread({ ...thread, messages: history.messages });
          }
        }
      })
      .catch((loadError: unknown) => {
        if (!cancelled) setError(loadError instanceof Error ? loadError.message : 'Could not restore this interview.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  const handleThreadChange = useCallback((thread: ChatThreadWithMessages) => {
    setActiveThread(thread);
  }, []);

  const finishInterview = async () => {
    if (!activeThread || finishing) return;
    setFinishing(true);
    setError(null);
    try {
      const result = await api.post<InterviewFinishResult>(
        `/chat/threads/${encodeURIComponent(activeThread.id)}/finish`,
        {},
      );
      setAssessment(result.assessment);
      setMemoryStatus(result.memory_update_status);
      setActiveThread((current) =>
        current ? { ...current, ...result.thread, messages: current.messages } : current,
      );
      setSession((current) => current ? { ...current, status: 'completed' } : current);
    } catch (finishError) {
      setError(finishError instanceof Error ? finishError.message : 'Could not finish the interview.');
    } finally {
      setFinishing(false);
    }
  };

  const exitInterview = async () => {
    if (activeThread) {
      await api.post(`/chat/threads/${encodeURIComponent(activeThread.id)}/exit`, {}).catch(() => {});
    }
    navigate('/dashboard');
  };

  const retryMemory = async () => {
    setFinishing(true);
    setError(null);
    try {
      await api.post('/memory/profile/refresh', {});
      setMemoryStatus('synced');
    } catch (refreshError) {
      setError(refreshError instanceof Error ? refreshError.message : 'Memory update is still unavailable.');
    } finally {
      setFinishing(false);
    }
  };

  if (loading) {
    return <WorkspacePage className="flex min-h-[70vh] items-center justify-center"><GridSpinner size="md" /></WorkspacePage>;
  }
  if (error && !session) {
    return (
      <WorkspacePage>
        <WorkspacePageHeader title="Interview" />
        <WorkspaceEmptyState
          icon={<MessageSquareText size={20} />}
          title="Interview unavailable"
          description={error}
          action={<Button asChild size="sm"><Link to="/dashboard">Back to dashboard</Link></Button>}
        />
      </WorkspacePage>
    );
  }
  if (!session) return null;
  const mode = sessionMode(session);
  const readOnlyThread = completedThread ?? (assessment && activeThread ? activeThread : null);

  return (
    <WorkspacePage className="flex min-h-[calc(100vh-1px)] flex-col">
      <WorkspacePageHeader
        title={session.title || 'Interview'}
        description={`${modeLabel(mode)} · Transcript saved with this session`}
        actions={
          <div className="flex items-center gap-2">
            {session.status === 'completed' ? (
              <Badge variant="outline"><CheckCircle2 className="text-green-700" />Completed</Badge>
            ) : (
              <Button variant="outline" size="sm" onClick={() => void exitInterview()}>
                <ArrowLeft data-icon="inline-start" />
                Save & exit
              </Button>
            )}
            {session.status !== 'completed' ? (
              <Button size="sm" onClick={() => void finishInterview()} disabled={!activeThread || finishing}>
                {finishing ? 'Finishing...' : 'Finish interview'}
              </Button>
            ) : null}
          </div>
        }
      />

      {error ? <p className="mb-3 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-950 dark:bg-amber-950/30 dark:text-amber-100">{error}</p> : null}

      {assessment ? (
        <section className="mb-4 grid gap-3 rounded-lg border p-4 sm:grid-cols-2">
          <div>
            <p className="text-sm font-medium">Strengths</p>
            <p className="text-muted-foreground mt-1 text-sm">{assessment.strengths.join(' · ') || 'No coarse signal yet'}</p>
          </div>
          <div>
            <p className="text-sm font-medium">Growth edges</p>
            <p className="text-muted-foreground mt-1 text-sm">{assessment.growth_edges.join(' · ') || 'No coarse signal yet'}</p>
          </div>
          {memoryStatus === 'failed' ? (
            <div className="sm:col-span-2 flex items-center justify-between gap-3 rounded-md border border-amber-300 px-3 py-2">
              <p className="text-sm">Interview saved. Memory update needs another attempt.</p>
              <Button variant="outline" size="sm" onClick={() => void retryMemory()} disabled={finishing}>
                <RefreshCw data-icon="inline-start" />Retry memory
              </Button>
            </div>
          ) : null}
        </section>
      ) : null}

      <div className="min-h-[560px] flex-1 overflow-hidden rounded-lg border">
        {session.status === 'completed' && !readOnlyThread ? (
          <WorkspaceEmptyState
            title="Transcript unavailable"
            description="This completed interview has no saved conversation."
            className="h-full"
          />
        ) : (
          <ChatPanel
            key={session.id}
            sessionId={session.id}
            kind="interview"
            mode={mode}
            initialThread={readOnlyThread ?? undefined}
            onThreadChange={handleThreadChange}
            readOnly={session.status === 'completed'}
          />
        )}
      </div>
    </WorkspacePage>
  );
}
