export type TaskStatus = "queued" | "processing" | "completed" | "failed";

export interface TaskAcceptedResponse {
  task_id: string;
  status: Exclude<TaskStatus, "failed">;
  cache_hit?: boolean;
  deduplicated?: boolean;
}

export interface TaskResponse {
  task_id: string;
  status: TaskStatus;
  result_url?: string;
  error?: string;
  cache_hit?: boolean;
  deduplicated?: boolean;
}

export interface TranscriptionResult {
  text: string;
}

export type SummaryStyle = "bullet_points" | "short_paragraph" | "executive";

export interface GleanIndexResult {
  status: "indexed";
  artifact_id: string;
  chunks_indexed: number;
}

export type ChatRole = "user" | "assistant";

export interface ChatMessage {
  id: string;
  role: ChatRole;
  content: string;
}

export interface StreamGleanAnswerOptions {
  documentId: string;
  query: string;
  signal?: AbortSignal;
  onToken: (token: string) => void;
}

export interface GleanIndexAcceptedResponse extends TaskAcceptedResponse {
  message: string;
  document_id: string;
}