"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Bot,
  FileSearch,
  RotateCcw,
  Send,
  Square,
  User,
} from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import ReactMarkdown from "react-markdown";
import { toast } from "sonner";
import { FilePicker } from "@/components/ui/file-picker";
import { NeoBadge } from "@/components/ui/neo-badge";
import { NeoButton } from "@/components/ui/neo-button";
import { NeoCard } from "@/components/ui/neo-card";
import { NeoInput } from "@/components/ui/neo-input";
import { PageIntro } from "@/components/ui/page-intro";
import { TaskProgress, TaskSkeleton } from "@/components/ui/task-progress";
import {
  getTask,
  indexDocument,
  streamGleanAnswer,
} from "@/lib/api";
import type { ChatMessage, GleanIndexAcceptedResponse, TaskResponse } from "@/lib/types";

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message.trim()
    ? error.message
    : fallback;
}

export default function GleanerPage() {
  const [document, setDocument] = useState<File | null>(null);
  const [taskId, setTaskId] = useState<string | null>(null);
  const [documentId, setDocumentId] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);

  const abortControllerRef = useRef<AbortController | null>(null);
  const lastNotifiedTask = useRef<string | null>(null);
  const lastTaskPollingError = useRef<string | null>(null);

  const indexMutation = useMutation<GleanIndexAcceptedResponse, Error, File>({
    mutationFn: indexDocument,
  });

  const taskQuery = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => getTask(taskId as string),
    enabled: Boolean(taskId),
    refetchInterval: (queryState: { state: { data?: TaskResponse } }) => {
      const task = queryState.state.data as TaskResponse | undefined;

      return task?.status === "completed" || task?.status === "failed"
        ? false
        : 1500;
    },
  });

  const task = taskQuery.data;

  const readyDocumentId =
  documentId ??
  (task?.status === "completed" && task.result_url ? taskId : null);

  useEffect(() => {
    if (!task || lastNotifiedTask.current === task.task_id) {
      return;
    }

    if (task.status === "failed") {
      toast.error("Document indexing failed", {
        description:
          "Please try again with the same file or choose a different document.",
      });
      lastNotifiedTask.current = task.task_id;
      return;
    }

    if (task.status === "completed" && !task.result_url) {
      toast.error("The indexed document could not be prepared", {
        description:
          "Processing finished, but the completed document was unavailable.",
      });
      lastNotifiedTask.current = task.task_id;
    }
  }, [task]);

  useEffect(() => {
    if (!taskQuery.isError) {
      lastTaskPollingError.current = null;
      return;
    }

    const message = getErrorMessage(
      taskQuery.error,
      "Lexos could not check the document's progress.",
    );

    if (lastTaskPollingError.current === message) {
      return;
    }

    lastTaskPollingError.current = message;

    toast.error("Indexing status could not be updated", {
      description: message,
    });
  }, [taskQuery.error, taskQuery.isError]);

  useEffect(() => {
    if (
      !taskId ||
      task?.status !== "completed" ||
      !task.result_url ||
      documentId === taskId ||
      lastNotifiedTask.current === task.task_id
    ) {
      return;
    }

    lastNotifiedTask.current = task.task_id;

    toast.success("Document indexed", {
      description: "You can now ask questions about its content.",
    });
  }, [documentId, task, taskId]);

  useEffect(() => {
    return () => abortControllerRef.current?.abort();
  }, []);

  async function handleIndex(): Promise<void> {
    if (indexMutation.isPending) {
      return;
    }

    if (!document) {
      toast.error("Choose a document first");
      return;
    }

    setTaskId(null);
    setDocumentId(null);
    setMessages([]);

    lastNotifiedTask.current = null;
    lastTaskPollingError.current = null;

    try {
      const acceptedIndex = await indexMutation.mutateAsync(document);

      setTaskId(acceptedIndex.document_id);
      if (acceptedIndex.status === "completed") {
        setDocumentId(acceptedIndex.document_id);
        lastNotifiedTask.current = acceptedIndex.task_id;
      }

      toast.success(
        acceptedIndex.cache_hit ? "Cached document index ready" : "Indexing started",
        {
          description: acceptedIndex.deduplicated
            ? "Existing indexing work was reused for identical document content."
            : "Lexos is preparing the document for questions.",
        },
      );
    } catch (error) {
      toast.error("Could not start indexing", {
        description: getErrorMessage(
          error,
          "An unexpected error prevented indexing.",
        ),
      });
    }
    
}

  async function ask(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();

    const trimmedQuery = query.trim();

    if (!readyDocumentId || !trimmedQuery || isStreaming) {
      return;
    }

    const userMessage: ChatMessage = {
      id: crypto.randomUUID(),
      role: "user",
      content: trimmedQuery,
    };

    const assistantMessage: ChatMessage = {
      id: crypto.randomUUID(),
      role: "assistant",
      content: "",
    };

    setMessages((current) => [
      ...current,
      userMessage,
      assistantMessage,
    ]);
    setQuery("");
    setIsStreaming(true);

    const controller = new AbortController();
    abortControllerRef.current = controller;

    try {
      await streamGleanAnswer({
        documentId: readyDocumentId,
        query: trimmedQuery,
        signal: controller.signal,
        onToken: (token) => {
          setMessages((current) =>
            current.map((message) =>
              message.id === assistantMessage.id
                ? {
                    ...message,
                    content: message.content + token,
                  }
                : message,
            ),
          );
        },
      });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        toast.info("Answer stopped");
      } else {
        const message = getErrorMessage(
          error,
          "The answer could not be completed.",
        );

        toast.error("Gleaner could not finish the answer", {
          description: message,
        });

        setMessages((current) =>
          current.map((item) =>
            item.id === assistantMessage.id && !item.content
              ? {
                  ...item,
                  content:
                    "I couldn't finish that response. Please ask the question again.",
                }
              : item,
          ),
        );
      }
    } finally {
      abortControllerRef.current = null;
      setIsStreaming(false);
    }
  }

  function stopStreaming(): void {
    abortControllerRef.current?.abort();
  }

  function reset(): void {
    abortControllerRef.current?.abort();
    indexMutation.reset();

    setDocument(null);
    setTaskId(null);
    setDocumentId(null);
    setQuery("");
    setMessages([]);

    lastNotifiedTask.current = null;
    lastTaskPollingError.current = null;
  }

  const hasSelectedDocument = document !== null;

  const hasActiveIndexTask =
    task?.status === "queued" || task?.status === "processing";

  const indexing =
    indexMutation.isPending || hasActiveIndexTask;

  const isIndexButtonDisabled =
    !hasSelectedDocument || indexing;

  return (
    <div className="pb-12">
      <PageIntro
        eyebrow="Service 03"
        title="Gleaner document RAG"
        description="Index a document and ask grounded questions through a real-time streamed RAG interface."
        badge={<NeoBadge variant="mint">Interactive RAG</NeoBadge>}
      />

      {!readyDocumentId ? (
        <div className="mx-auto max-w-4xl">
          <NeoCard accent="purple" className="p-6 md:p-8">
            <div className="mb-7 flex items-center gap-4">
              <span className="grid size-14 place-items-center border-[3px] border-white bg-purple-500 text-black">
                <FileSearch className="size-7" strokeWidth={3} />
              </span>

              <div>
                <NeoBadge variant="purple">Phase 1</NeoBadge>
                <h2 className="mt-2 text-3xl font-black uppercase tracking-[-0.04em]">
                  Index document
                </h2>
              </div>
            </div>

            <FilePicker
              label="Choose document to index"
              hint="Upload the document you want Lexos to understand and answer questions about."
              file={document}
              onChange={setDocument}
              disabled={indexing}
            />

            <div className="mt-6 flex flex-wrap gap-4">
              <NeoButton
                onClick={() => void handleIndex()}
                disabled={isIndexButtonDisabled}
                aria-busy={indexing}
              >
                <Send className="size-4" />

                {indexMutation.isPending
                  ? "Starting"
                  : hasActiveIndexTask
                    ? "Indexing"
                    : "Index"}
              </NeoButton>

              <NeoButton
                variant="white"
                onClick={reset}
                disabled={indexMutation.isPending}
              >
                <RotateCcw className="size-4" />
                Reset
              </NeoButton>
            </div>

            {taskQuery.isPending && taskId && <TaskSkeleton />}

            {taskQuery.isError && (
              <p className="mt-6 border-[3px] border-red-400 bg-red-950 p-4 font-mono text-sm text-red-200">
                {getErrorMessage(
                  taskQuery.error,
                  "The document's progress could not be checked.",
                )}
              </p>
            )}

            {task && <TaskProgress task={task} />}
          </NeoCard>
        </div>
      ) : (
        <div className="grid gap-8 xl:grid-cols-[320px_minmax(0,1fr)]">
          <NeoCard accent="purple" className="h-fit p-6">
            <NeoBadge variant="mint">Phase 2</NeoBadge>

            <h2 className="mt-4 text-3xl font-black uppercase tracking-[-0.04em]">
              Ask your document
            </h2>

            <div className="mt-6 border-[3px] border-green-400 bg-zinc-950 p-4">
              <p className="font-mono text-[10px] font-black uppercase tracking-[0.16em] text-zinc-500">
                Indexed document
              </p>

              <p className="mt-2 break-words font-mono text-xs text-green-400">
                {document?.name ?? "Document ready"}
              </p>
            </div>

            <div className="mt-5 space-y-3 font-mono text-xs leading-5 text-zinc-400">
              <p>
                Ask specific questions about facts, themes, decisions, or
                details.
              </p>
              <p>
                Each answer is generated from the document you indexed.
              </p>
            </div>

            <NeoButton
              variant="white"
              className="mt-6 w-full"
              onClick={reset}
            >
              <RotateCcw className="size-4" />
              New document
            </NeoButton>
          </NeoCard>

          <NeoCard
            accent="mint"
            className="flex h-[640px] flex-col overflow-hidden p-0 md:h-[680px] xl:h-[calc(100vh-8rem)] xl:max-h-[820px]"
          >
            <div className="flex shrink-0 items-center justify-between border-b-[3px] border-white bg-zinc-950 px-5 py-4">
              <div className="flex items-center gap-3">
                <Bot className="size-5 text-green-400" />
                <span className="font-mono text-xs font-black uppercase tracking-[0.14em]">
                  Live answers
                </span>
              </div>

              <NeoBadge variant={isStreaming ? "purple" : "mint"}>
                {isStreaming ? "Answering" : "Ready"}
              </NeoBadge>
            </div>

            <div className="min-h-0 flex-1 space-y-5 overflow-y-auto overscroll-contain bg-zinc-900 p-5 md:p-7">
              {messages.length === 0 ? (
                <div className="flex min-h-[470px] flex-col items-center justify-center text-center text-zinc-600">
                  <Bot className="mb-5 size-14" strokeWidth={1.5} />

                  <p className="font-black uppercase tracking-[0.15em] text-zinc-500">
                    Ask the indexed document
                  </p>

                  <p className="mt-3 max-w-md font-mono text-xs leading-5">
                    Ask a question and Lexos will answer using the indexed
                    document as its source.
                  </p>
                </div>
              ) : (
                messages.map((message) => (
                  <article
                    key={message.id}
                    className={`border-[3px] p-4 md:p-5 ${
                      message.role === "user"
                        ? "ml-auto max-w-3xl border-white bg-zinc-800 shadow-[4px_4px_0px_0px_rgba(255,255,255,1)]"
                        : "mr-auto max-w-4xl border-purple-500 bg-zinc-950 shadow-[4px_4px_0px_0px_rgba(168,85,247,1)]"
                    }`}
                  >
                    <div className="mb-3 flex items-center gap-2 font-mono text-[10px] font-black uppercase tracking-[0.16em] text-zinc-500">
                      {message.role === "user" ? (
                        <User className="size-4 text-white" />
                      ) : (
                        <Bot className="size-4 text-green-400" />
                      )}

                      {message.role === "user" ? "You" : "Gleaner"}
                    </div>

                    {message.role === "assistant" ? (
                      message.content ? (
                        <div className="markdown-output font-mono text-sm">
                          <ReactMarkdown>
                            {message.content}
                          </ReactMarkdown>
                        </div>
                      ) : (
                        <span className="inline-block h-5 w-3 animate-pulse bg-green-400" />
                      )
                    ) : (
                      <p className="whitespace-pre-wrap font-mono text-sm leading-6 text-zinc-100">
                        {message.content}
                      </p>
                    )}
                  </article>
                ))
              )}
            </div>

            <form
              onSubmit={ask}
              className="shrink-0 border-t-[3px] border-white bg-zinc-950 p-4 md:p-5"
            >
              <div className="flex flex-col gap-4 md:flex-row">
                <NeoInput
                  value={query}
                  disabled={isStreaming}
                  placeholder="Ask a question about the indexed document..."
                  aria-label="Question for Gleaner"
                  onChange={(event) => setQuery(event.target.value)}
                />

                {isStreaming ? (
                  <NeoButton
                    type="button"
                    variant="danger"
                    onClick={stopStreaming}
                  >
                    <Square className="size-4 fill-current" />
                    Stop
                  </NeoButton>
                ) : (
                  <NeoButton type="submit" disabled={!query.trim()}>
                    <Send className="size-4" />
                    Ask
                  </NeoButton>
                )}
              </div>
            </form>
          </NeoCard>
        </div>
      )}
    </div>
  );
}