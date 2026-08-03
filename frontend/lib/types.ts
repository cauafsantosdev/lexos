export type TaskStatus = "queued" | "processing" | "completed" | "failed";

export interface TaskAcceptedResponse {
  task_id: string;
  status: "queued";
}

export interface TaskResponse {
  task_id: string;
  status: TaskStatus;
  result_url: string;
}

export interface TranscriptionResult {
  text: string;
}

export type SummaryStyle = "bullet_points" | "short_paragraph" | "executive";

export interface GleanIndexResult {
  document_id: string;
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

export interface GleanIndexAcceptedResponse {
  message: string;
  document_id: string;
  status: "queued";
}