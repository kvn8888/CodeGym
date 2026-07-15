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
  test_config: { strategy: string };
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
  stderr?: string;
  stdout?: string;
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

export interface Submission {
  id: string;
  problem_id: string;
  status: string;
  language: string;
  submitted_at: string;
  completed_at?: string;
  duration_ms?: number;
  total_tests?: number;
  passed_tests?: number;
  result?: TestResult;
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
export type PracticeFormat = 'mcq' | 'coding';

export interface NewPracticeConfig {
  /** `mcq` = multiple-choice marathon; `coding` = DSA / LeetCode-style workspace problem. */
  format?: PracticeFormat;
  prompt: string;
  difficulty: 'easy' | 'medium' | 'hard';
  count: number;
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
