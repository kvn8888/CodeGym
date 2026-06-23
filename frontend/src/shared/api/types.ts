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
