export interface ProblemSummary {
  id: string;
  title: string;
  category: string;
  language: string;
  framework?: string;
  difficulty: number;
  tags: string[];
  estimated_minutes: number;
  type: string;
}

export interface Problem extends ProblemSummary {
  version: string;
  description: string;
  subcategory?: string;
  runtime: RuntimeConfig;
  files: { skeleton: FileRef[] };
  test_config: {
    strategy: 'unit' | 'http';
    readiness_timeout_seconds?: number;
  };
  hints?: Hint[];
}

export interface RuntimeConfig {
  image: string;
  timeout_seconds: number;
  memory_mb: number;
  network_mode: string;
}

export interface FileRef {
  path: string;
  entry?: boolean;
  readonly?: boolean;
}

export interface Hint {
  cost: number;
  text: string;
}

export interface SubmissionFile {
  path: string;
  content: string;
}

export interface TestResult {
  status: string;
  total: number;
  passed: number;
  failed: number;
  duration_ms: number;
  test_cases: TestCaseResult[];
  compile_error?: string;
}

export interface TestCaseResult {
  name: string;
  status: string;
  duration_ms: number;
  expected?: string;
  actual?: string;
  output?: string;
  error?: string;
}

export type SubmissionStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'timeout'
  | 'out_of_memory'
  | 'crashed'
  | 'error';

export interface Submission {
  status: SubmissionStatus;
  result?: TestResult;
  failure_detail?: string;
  stdout: string;
  output_truncated: boolean;
}

export interface UserProfile {
  user_id: string;
  email: string;
  display_name: string;
  display_name_source: 'oauth' | 'user' | 'fallback';
  default_workspace_id: string;
}

export interface UpdateUserProfileInput {
  display_name: string;
}

export interface UserMemoryProfile {
  summary: string;
  updated_at: string;
  next_review_at: string;
  strengths: string[];
  growth_edges: string[];
  skills: SkillProficiency[];
  notes: MemoryNote[];
}

export interface SkillProficiency {
  id: string;
  label: string;
  area: string;
  level: number;
  confidence: number;
  trend: 'up' | 'flat' | 'down';
  last_practiced: string;
}

export interface MemoryNote {
  id: string;
  problem_id: string;
  title: string;
  summary: string;
  created_at: string;
  tags: string[];
  action: 'keep' | 'review' | 'prune';
}

export interface MemoryEvent {
  id: string;
  workspace_id: string;
  user_id: string;
  source: string;
  type: string;
  summary: string;
  payload?: unknown;
  occurred_at: string;
  created_at: string;
}

export type SessionKind = 'workspace' | 'mcq' | 'interview';
export type SessionStatus = 'active' | 'completed' | 'abandoned';

export interface PracticeSessionSummary {
  id: string;
  workspace_id: string;
  user_id: string;
  kind: SessionKind;
  status: SessionStatus;
  title: string;
  problem_id?: string;
  generation_job_id?: string;
  created_at: string;
  updated_at: string;
  last_activity_at: string;
  completed_at?: string;
}

export interface PracticeSession extends PracticeSessionSummary {
  state: unknown;
  files?: Array<{
    file_path: string;
    content: string;
    updated_at: string;
  }>;
}

/** Practice format chosen on New practice. */
export type PracticeFormat = 'mcq' | 'coding' | 'interview';
export type ProblemLanguage = 'python' | 'go';
export type InterviewMode = 'coding' | 'system_design' | 'behavioral' | 'open_coaching';
export type MCQQuestionType = 'single_select' | 'multi_select' | 'free_response';

export interface NewPracticeConfig {
  /** Chosen New Practice surface. */
  format?: PracticeFormat;
  interviewMode?: InterviewMode;
  prompt: string;
  difficulty: 'easy' | 'medium' | 'hard';
  count: number;
  language?: ProblemLanguage;
  intakeId?: string;
}

export type ChatKind = 'interview' | 'coach';
export type ChatThreadStatus = 'active' | 'closed' | 'completed';
export type ChatMessageStatus = 'complete' | 'interrupted';

export interface ChatContextEnvelope {
  version: 1;
  session_id: string;
  problem_id?: string;
  question_id?: string;
}

export interface ChatThread {
  id: string;
  workspace_id: string;
  user_id: string;
  session_id: string;
  kind: ChatKind;
  mode?: InterviewMode;
  status: ChatThreadStatus;
  context: ChatContextEnvelope;
  successor_id?: string;
  created_at: string;
  updated_at: string;
  closed_at?: string;
}

export interface ChatMessage {
  id: string;
  thread_id: string;
  role: 'user' | 'assistant';
  content: string;
  status: ChatMessageStatus;
  client_message_id?: string;
  reply_to_message_id?: string;
  created_at: string;
}

export interface ChatThreadWithMessages extends ChatThread {
  messages: ChatMessage[];
}

export interface InterviewAssessment {
  strengths: string[];
  growth_edges: string[];
  topic: string;
  mode: InterviewMode;
  turn_count: number;
  duration_seconds: number;
}

export interface InterviewFinishResult {
  thread: ChatThread;
  assessment: InterviewAssessment;
  memory_update_status: 'synced' | 'failed';
}

export type PracticeIntakeStatus = 'pending' | 'completed' | 'skipped';

export interface PracticeIntakeOption {
  id: string;
  label: string;
}

export interface PracticeIntakeQuestion {
  id: string;
  dimension: string;
  text: string;
  options: PracticeIntakeOption[];
}

export interface PracticeIntake {
  id: string;
  workspace_id: string;
  user_id: string;
  normalized_topic: string;
  original_topic: string;
  practice_seed: Record<string, unknown>;
  questions: PracticeIntakeQuestion[];
  answers: Record<string, string>;
  status: PracticeIntakeStatus;
  suppression_reason?: string;
  generation_error?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

/** Aggregate from GET /api/v1/cost — estimated GenAI spend for this workspace user. */
export interface GenAICostAggregate {
  currency: string;
  pricing_as_of: string;
  total_tokens_in: number;
  total_tokens_out: number;
  total_cost_usd: number;
  call_count: number;
  by_provider: Array<{
    provider: string;
    tokens_in: number;
    tokens_out: number;
    cost_usd: number;
    call_count: number;
  }>;
  by_model: Array<{
    provider: string;
    model: string;
    tokens_in: number;
    tokens_out: number;
    cost_usd: number;
    call_count: number;
  }>;
}
