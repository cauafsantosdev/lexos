"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { AudioLines, RotateCcw, Send, SquareTerminal } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { FilePicker } from "@/components/ui/file-picker";
import { NeoBadge } from "@/components/ui/neo-badge";
import { NeoButton } from "@/components/ui/neo-button";
import { NeoCard } from "@/components/ui/neo-card";
import { PageIntro } from "@/components/ui/page-intro";
import { TaskProgress, TaskSkeleton } from "@/components/ui/task-progress";
import { fetchTranscriptionResult, getTask, transcribeAudio } from "@/lib/api";
import type { TaskAcceptedResponse, TaskResponse } from "@/lib/types";

export default function ScriberPage() {
  const [audio, setAudio] = useState<File | null>(null);
  const [taskId, setTaskId] = useState<string | null>(null);
  const lastNotifiedTask = useRef<string | null>(null);

  const mutation = useMutation<TaskAcceptedResponse, Error, File>({
    mutationFn: transcribeAudio,
    onSuccess: (task: TaskAcceptedResponse) => {
      setTaskId(task.task_id);
      lastNotifiedTask.current = task.cache_hit ? task.task_id : null;
      toast.success(task.cache_hit ? "Cached transcription ready" : "Transcription started", {
        description: task.deduplicated
          ? "Existing processing was reused for identical audio."
          : undefined,
      });
    },
    onError: (error: Error) => {
      toast.error("Could not start transcription", {
        description: error.message,
      });
    },
  });

  const taskQuery = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => getTask(taskId as string),
    enabled: Boolean(taskId),
    refetchInterval: (query: { state: { data?: TaskResponse } }) => {
      const task = query.state.data as TaskResponse | undefined;
      return task?.status === "completed" || task?.status === "failed"
        ? false
        : 1500;
    },
  });

  const task = taskQuery.data;

  const resultQuery = useQuery({
    queryKey: ["transcription-result", task?.result_url],
    queryFn: () => fetchTranscriptionResult(task?.result_url as string),
    enabled: task?.status === "completed" && Boolean(task.result_url),
    staleTime: Number.POSITIVE_INFINITY,
  });

  useEffect(() => {
    if (!task || lastNotifiedTask.current === task.task_id) {
      return;
    }

    if (task.status === "completed") {
      toast.success("Transcription completed");
      lastNotifiedTask.current = task.task_id;
    }

    if (task.status === "failed") {
      toast.error("Transcription failed");
      lastNotifiedTask.current = task.task_id;
    }
  }, [task]);

  function submit(): void {
    if (!audio) {
      toast.error("Select an audio file first");
      return;
    }

    mutation.mutate(audio);
  }

  function reset(): void {
    setAudio(null);
    setTaskId(null);
    lastNotifiedTask.current = null;
  }

  const isBusy =
    mutation.isPending ||
    task?.status === "queued" ||
    task?.status === "processing";

  return (
    <div className="pb-12">
      <PageIntro
        eyebrow="Service 01"
        title="Scriber transcription"
        description="Upload an audio file and watch as AI transcribe your speech to text."
        badge={<NeoBadge variant="mint">Speech-to-Text</NeoBadge>}
      />

      <div className="grid gap-8 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <NeoCard accent="purple" className="h-fit p-6 md:p-8">
          <div className="mb-6 flex items-center gap-3">
            <span className="grid size-12 place-items-center border-[3px] border-white bg-purple-500 text-black">
              <AudioLines className="size-6" strokeWidth={3} />
            </span>

            <div>
              <h2 className="text-2xl font-black uppercase tracking-[-0.03em]">
                Audio input
              </h2>
              <p className="font-mono text-xs text-zinc-500">
                Upload one file to begin
              </p>
            </div>
          </div>

          <FilePicker
            label="Choose audio file"
            hint="Select the recording you want Lexos to transcribe."
            accept="audio/*"
            file={audio}
            onChange={setAudio}
            disabled={isBusy}
          />

          <div className="mt-6 flex flex-wrap gap-4">
            <NeoButton onClick={submit} disabled={!audio || isBusy}>
              <Send className="size-4" />
              {mutation.isPending ? "Starting" : "Transcribe"}
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

        <NeoCard accent="mint" className="min-h-[520px] p-0">
          <div className="flex items-center justify-between border-b-[3px] border-white bg-zinc-950 px-5 py-4">
            <div className="flex items-center gap-3">
              <SquareTerminal className="size-5 text-green-400" />
              <span className="font-mono text-xs font-black uppercase tracking-[0.14em]">
                Transcript
              </span>
            </div>

            <NeoBadge variant={resultQuery.data ? "mint" : "white"}>
              {resultQuery.data ? "Ready" : "Waiting"}
            </NeoBadge>
          </div>

          <div className="min-h-[460px] bg-black p-6 font-mono text-sm leading-7 text-zinc-200 md:p-8">
            {resultQuery.isPending && task?.status === "completed" ? (
              <div className="space-y-3">
                {Array.from({ length: 9 }, (_, index) => (
                  <div
                    key={index}
                    className="h-4 animate-pulse bg-zinc-800"
                    style={{ width: `${92 - ((index * 11) % 35)}%` }}
                  />
                ))}
              </div>
            ) : resultQuery.isError ? (
              <p className="text-red-400">{resultQuery.error.message}</p>
            ) : resultQuery.data ? (
              <pre className="whitespace-pre-wrap font-mono">
                {resultQuery.data.text}
              </pre>
            ) : (
              <div className="flex min-h-[380px] flex-col items-center justify-center text-center text-zinc-600">
                <AudioLines className="mb-5 size-12" strokeWidth={1.5} />

                <p className="font-black uppercase tracking-[0.15em] text-zinc-500">
                  No transcript yet
                </p>

                <p className="mt-3 max-w-sm text-xs leading-5">
                  Your transcript will appear here as soon as processing finishes.
                </p>
              </div>
            )}
          </div>
        </NeoCard>
      </div>
    </div>
  );
}