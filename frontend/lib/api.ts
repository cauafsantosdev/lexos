import type {
  GleanIndexAcceptedResponse,
  GleanIndexResult,
  StreamGleanAnswerOptions,
  SummaryStyle,
  TaskAcceptedResponse,
  TaskResponse,
  TranscriptionResult,
} from "@/lib/types";

const API_URL = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8000").replace(
  /\/+$/,
  "",
);

interface DistillerResult {
  summary: string;
}

function isAbsoluteUrl(value: string): boolean {
  return /^https?:\/\//i.test(value);
}

function resolveBackendUrl(pathOrUrl: string): string {
  if (isAbsoluteUrl(pathOrUrl)) {
    return pathOrUrl;
  }

  const path = pathOrUrl.startsWith("/") ? pathOrUrl : `/${pathOrUrl}`;
  return `${API_URL}${path}`;
}

function isDistillerResult(value: unknown): value is DistillerResult {
  return (
    typeof value === "object" &&
    value !== null &&
    "summary" in value &&
    typeof value.summary === "string"
  );
}

function isGleanIndexAcceptedResponse(
  value: unknown,
): value is GleanIndexAcceptedResponse {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const payload = value as Record<string, unknown>;

  return (
    typeof payload.task_id === "string" &&
    payload.task_id.length > 0 &&
    typeof payload.document_id === "string" &&
    payload.document_id.length > 0 &&
    (payload.status === "queued" ||
      payload.status === "processing" ||
      payload.status === "completed") &&
    typeof payload.message === "string"
  );
}

async function readError(response: Response): Promise<string> {
  const body = await response.text();
  return body.trim() || `${response.status} ${response.statusText}`;
}

async function requestJson<T>(pathOrUrl: string, init?: RequestInit): Promise<T> {
  const response = await fetch(resolveBackendUrl(pathOrUrl), {
    ...init,
    headers: {
      Accept: "application/json",
      ...init?.headers,
    },
  });

  if (!response.ok) {
    throw new Error(await readError(response));
  }

  return (await response.json()) as T;
}

export async function transcribeAudio(audio: File): Promise<TaskAcceptedResponse> {
  const formData = new FormData();
  formData.append("audio", audio);

  return requestJson<TaskAcceptedResponse>("/transcribe", {
    method: "POST",
    body: formData,
  });
}

export async function summarizeFile(
  document: File,
  style: SummaryStyle,
): Promise<TaskAcceptedResponse> {
  const formData = new FormData();
  formData.append("document", document);
  formData.append("style", style);

  return requestJson<TaskAcceptedResponse>("/summarize", {
    method: "POST",
    body: formData,
  });
}

export async function summarizeText(
  documentText: string,
  style: SummaryStyle,
): Promise<TaskAcceptedResponse> {
  return requestJson<TaskAcceptedResponse>("/summarize", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      document_text: documentText,
      style,
    }),
  });
}

export async function indexDocument(
  document: File,
): Promise<GleanIndexAcceptedResponse> {
  const formData = new FormData();
  formData.append("document", document);

  const payload = await requestJson<unknown>("/glean/index", {
    method: "POST",
    body: formData,
  });

  if (!isGleanIndexAcceptedResponse(payload)) {
    console.error("Invalid Gleaner indexing response:", payload);

    throw new Error(
      "Lexos accepted the document but could not start tracking its progress.",
    );
  }

  return payload;
}

export async function getTask(taskId: string): Promise<TaskResponse> {
  return requestJson<TaskResponse>(`/task/${encodeURIComponent(taskId)}`);
}

export async function fetchTranscriptionResult(
  resultUrl: string,
): Promise<TranscriptionResult> {
  return requestJson<TranscriptionResult>(resultUrl);
}

export async function fetchGleanIndexResult(resultUrl: string): Promise<GleanIndexResult> {
  return requestJson<GleanIndexResult>(resultUrl);
}

export async function fetchResultText(resultUrl: string): Promise<string> {
  const result = await requestJson<unknown>(resultUrl);

  if (!isDistillerResult(result)) {
    throw new Error("The completed summary did not contain a valid summary field.");
  }

  return result.summary;
}

function parseSseBlock(block: string, onToken: (token: string) => void): boolean {
  const dataLines = block
    .split("\n")
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trimStart());

  if (dataLines.length === 0) {
    return false;
  }

  const data = dataLines.join("\n");
  if (data === "[DONE]") {
    return true;
  }

  const payload: unknown = JSON.parse(data);
  if (
    typeof payload !== "object" ||
    payload === null ||
    !("token" in payload) ||
    typeof payload.token !== "string"
  ) {
    throw new Error("The answer stream returned an invalid token.");
  }

  onToken(payload.token);
  return false;
}

export async function streamGleanAnswer({
  documentId,
  query,
  signal,
  onToken,
}: StreamGleanAnswerOptions): Promise<void> {
  const url = new URL(resolveBackendUrl("/glean/ask"));
  url.searchParams.set("document_id", documentId);
  url.searchParams.set("query", query);

  const response = await fetch(url.toString(), {
    method: "GET",
    headers: {
      Accept: "text/event-stream",
    },
    signal,
  });

  if (!response.ok) {
    throw new Error(await readError(response));
  }

  if (!response.body) {
    throw new Error("The answer stream could not be read by this browser.");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done }).replace(/\r\n/g, "\n");

      let boundaryIndex = buffer.indexOf("\n\n");
      while (boundaryIndex !== -1) {
        const block = buffer.slice(0, boundaryIndex);
        buffer = buffer.slice(boundaryIndex + 2);

        if (parseSseBlock(block, onToken)) {
          await reader.cancel();
          return;
        }

        boundaryIndex = buffer.indexOf("\n\n");
      }

      if (done) {
        if (buffer.trim() && parseSseBlock(buffer, onToken)) {
          return;
        }
        return;
      }
    }
  } finally {
    reader.releaseLock();
  }
}