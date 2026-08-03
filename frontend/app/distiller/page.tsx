"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import {
  FileText,
  RotateCcw,
  Send,
  Sparkles,
  TextCursorInput,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import { toast } from "sonner";
import { FilePicker } from "@/components/ui/file-picker";
import { NeoBadge } from "@/components/ui/neo-badge";
import { NeoButton } from "@/components/ui/neo-button";
import { NeoCard } from "@/components/ui/neo-card";
import { NeoSelect, NeoTextarea } from "@/components/ui/neo-input";
import { PageIntro } from "@/components/ui/page-intro";
import { TaskProgress, TaskSkeleton } from "@/components/ui/task-progress";
import {
  fetchResultText,
  getTask,
  summarizeFile,
  summarizeText,
} from "@/lib/api";
import type { SummaryStyle, TaskResponse } from "@/lib/types";

type InputMode = "file" | "text";

const styleLabels: Record<SummaryStyle, string> = {
  bullet_points: "Bullet points",
  short_paragraph: "Short paragraph",
  executive: "Executive",
};

export default function DistillerPage() {
  const [mode, setMode] = useState<InputMode>("file");
  const [document, setDocument] = useState<File | null>(null);
  const [documentText, setDocumentText] = useState("");
  const [style, setStyle] = useState<SummaryStyle>("bullet_points");
  const [taskId, setTaskId] = useState<string | null>(null);
  const lastNotifiedTask = useRef<string | null>(null);

  const mutation = useMutation({
    mutationFn: async () => {
      if (mode === "file") {
        if (!document) {
          throw new Error("Select a document before submitting.");
        }

        return summarizeFile(document, style);
      }

      const trimmedText = documentText.trim();

      if (!trimmedText) {
        throw new Error("Paste document text before submitting.");
      }

      return summarizeText(trimmedText, style);
    },
    onSuccess: (task) => {
      setTaskId(task.task_id);
      lastNotifiedTask.current = null;
      toast.success("Summary started");
    },
    onError: (error) => {
      toast.error("Could not start summary", {
        description: error.message,
      });
    },
  });

  const taskQuery = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => getTask(taskId as string),
    enabled: Boolean(taskId),
    refetchInterval: (query) => {
      const task = query.state.data as TaskResponse | undefined;
      return task?.status === "completed" || task?.status === "failed"
        ? false
        : 1500;
    },
  });

  const task = taskQuery.data;

  const summaryQuery = useQuery({
    queryKey: ["summary-result", task?.result_url],
    queryFn: () => fetchResultText(task?.result_url as string),
    enabled: task?.status === "completed" && Boolean(task.result_url),
    staleTime: Number.POSITIVE_INFINITY,
  });

  useEffect(() => {
    if (!task || lastNotifiedTask.current === task.task_id) {
      return;
    }

    if (task.status === "completed") {
      toast.success("Summary completed");
      lastNotifiedTask.current = task.task_id;
    }

    if (task.status === "failed") {
      toast.error("Summary failed");
      lastNotifiedTask.current = task.task_id;
    }
  }, [task]);

  function reset(): void {
    setDocument(null);
    setDocumentText("");
    setTaskId(null);
    lastNotifiedTask.current = null;
  }

  const hasInput =
    mode === "file" ? Boolean(document) : Boolean(documentText.trim());

  const isBusy =
    mutation.isPending ||
    task?.status === "queued" ||
    task?.status === "processing";

  return (
    <div className="pb-12">
      <PageIntro
        eyebrow="Service 02"
        title="Distiller summarization"
        description="Submit a document or paste raw text, select your desired format, and let the AI extract the core insights."
        badge={<NeoBadge variant="mint">Document Analysis</NeoBadge>}
      />

      <div className="grid gap-8 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]">
        <NeoCard accent="purple" className="h-fit p-6 md:p-8">
          <div className="mb-6 grid grid-cols-2 gap-3">
            <NeoButton
              variant={mode === "file" ? "purple" : "white"}
              onClick={() => setMode("file")}
              disabled={isBusy}
            >
              <FileText className="size-4" />
              File
            </NeoButton>

            <NeoButton
              variant={mode === "text" ? "mint" : "white"}
              onClick={() => setMode("text")}
              disabled={isBusy}
            >
              <TextCursorInput className="size-4" />
              Text
            </NeoButton>
          </div>

          <label className="mb-6 block">
            <span className="mb-2 block font-mono text-xs font-black uppercase tracking-[0.14em] text-zinc-300">
              Summary style
            </span>

            <NeoSelect
              value={style}
              disabled={isBusy}
              onChange={(event) =>
                setStyle(event.target.value as SummaryStyle)
              }
            >
              {(Object.keys(styleLabels) as SummaryStyle[]).map((value) => (
                <option key={value} value={value}>
                  {styleLabels[value]}
                </option>
              ))}
            </NeoSelect>
          </label>

          {mode === "file" ? (
            <FilePicker
              label="Choose document"
              hint="Upload the document you want Lexos to summarize."
              file={document}
              onChange={setDocument}
              disabled={isBusy}
              accent="purple"
            />
          ) : (
            <label className="block">
              <span className="mb-2 block font-mono text-xs font-black uppercase tracking-[0.14em] text-zinc-300">
                Document text
              </span>

              <NeoTextarea
                value={documentText}
                disabled={isBusy}
                placeholder="Paste the full document text here..."
                onChange={(event) => setDocumentText(event.target.value)}
              />

              <span className="mt-3 block font-mono text-[11px] leading-5 text-zinc-500">
                Paste the source text exactly as you want it analyzed.
              </span>
            </label>
          )}

          <div className="mt-6 flex flex-wrap gap-4">
            <NeoButton
              onClick={() => mutation.mutate()}
              disabled={!hasInput || isBusy}
            >
              <Send className="size-4" />
              {mutation.isPending ? "Starting" : "Distill"}
            </NeoButton>

            <NeoButton
              variant="white"
              onClick={reset}
              disabled={mutation.isPending}
            >
              <RotateCcw className="size-4" />
              Reset
            </NeoButton>
          </div>

          {taskQuery.isPending && taskId && <TaskSkeleton />}

          {taskQuery.isError && (
            <p className="mt-6 border-[3px] border-red-400 bg-red-950 p-4 font-mono text-sm text-red-200">
              {taskQuery.error.message}
            </p>
          )}

          {task && <TaskProgress task={task} />}
        </NeoCard>

        <NeoCard accent="mint" className="min-h-[620px] p-0">
          <div className="flex items-center justify-between border-b-[3px] border-white bg-zinc-950 px-5 py-4">
            <div className="flex items-center gap-3">
              <Sparkles className="size-5 text-green-400" />
              <span className="font-mono text-xs font-black uppercase tracking-[0.14em]">
                Summary
              </span>
            </div>

            <NeoBadge variant={summaryQuery.data ? "mint" : "white"}>
              {summaryQuery.data ? styleLabels[style] : "Waiting"}
            </NeoBadge>
          </div>

          <div className="min-h-[560px] p-6 md:p-8">
            {summaryQuery.isPending && task?.status === "completed" ? (
              <div className="space-y-4">
                <div className="h-8 w-2/3 animate-pulse bg-zinc-700" />

                {Array.from({ length: 10 }, (_, index) => (
                  <div
                    key={index}
                    className="h-4 animate-pulse bg-zinc-700"
                    style={{ width: `${96 - ((index * 13) % 42)}%` }}
                  />
                ))}
              </div>
            ) : summaryQuery.isError ? (
              <p className="border-[3px] border-red-400 bg-red-950 p-4 font-mono text-sm text-red-200">
                {summaryQuery.error.message}
              </p>
            ) : summaryQuery.data ? (
              <article className="markdown-output">
                <ReactMarkdown>{summaryQuery.data}</ReactMarkdown>
              </article>
            ) : (
              <div className="flex min-h-[500px] flex-col items-center justify-center text-center text-zinc-600">
                <FileText className="mb-5 size-12" strokeWidth={1.5} />

                <p className="font-black uppercase tracking-[0.15em] text-zinc-500">
                  No summary yet
                </p>

                <p className="mt-3 max-w-md font-mono text-xs leading-5">
                  Your formatted summary will appear here when the analysis is
                  complete.
                </p>
              </div>
            )}
          </div>
        </NeoCard>
      </div>
    </div>
  );
}
