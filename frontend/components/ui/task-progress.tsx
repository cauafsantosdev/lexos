import {
  Check,
  CircleAlert,
  LoaderCircle,
  Timer,
} from "lucide-react";
import { NeoBadge } from "@/components/ui/neo-badge";
import { NeoCard } from "@/components/ui/neo-card";
import type { TaskResponse, TaskStatus } from "@/lib/types";

const statusMeta: Record<
  TaskStatus,
  {
    label: string;
    badge: "white" | "warning" | "mint" | "danger" | "purple";
    icon: typeof Timer;
    title: string;
    description: string;
  }
> = {
  queued: {
    label: "Waiting",
    badge: "warning",
    icon: Timer,
    title: "Your file is in line",
    description: "Processing will begin automatically in a moment.",
  },
  processing: {
    label: "In progress",
    badge: "purple",
    icon: LoaderCircle,
    title: "Your document is being processed",
    description: "You can keep this page open while Lexos prepares the result.",
  },
  completed: {
    label: "Ready",
    badge: "mint",
    icon: Check,
    title: "Your result is ready",
    description: "The finished content is available below.",
  },
  failed: {
    label: "Something went wrong",
    badge: "danger",
    icon: CircleAlert,
    title: "We could not process this file",
    description: "Please try again or choose a different file.",
  },
};

export function TaskProgress({ task }: { task: TaskResponse }) {
  const meta = statusMeta[task.status];
  const Icon = meta.icon;

  return (
    <NeoCard
      accent={task.status === "failed" ? "white" : "purple"}
      className="mt-6"
    >
      <div className="flex items-start gap-4">
        <span
          className={`grid size-12 shrink-0 place-items-center border-[3px] border-white ${
            task.status === "completed"
              ? "bg-green-400 text-black"
              : task.status === "failed"
                ? "bg-red-400 text-black"
                : task.status === "processing"
                  ? "bg-purple-500 text-black"
                  : "bg-zinc-950 text-white"
          }`}
        >
          <Icon
            className={
              task.status === "processing"
                ? "size-6 animate-spin"
                : "size-6"
            }
            strokeWidth={3}
          />
        </span>

        <div className="min-w-0">
          <NeoBadge variant={meta.badge}>{meta.label}</NeoBadge>

          <h3 className="mt-3 text-lg font-black uppercase tracking-[-0.02em] text-white">
            {meta.title}
          </h3>

          <p className="mt-1 text-sm leading-6 text-zinc-400">
            {meta.description}
          </p>
        </div>
      </div>

      {(task.status === "queued" || task.status === "processing") && (
        <div className="mt-5 h-3 overflow-hidden border-[3px] border-white bg-zinc-950">
          <div className="h-full w-1/3 animate-[progress_1.4s_ease-in-out_infinite] bg-green-400" />
        </div>
      )}
    </NeoCard>
  );
}

export function TaskSkeleton() {
  return (
    <div className="mt-6 border-[3px] border-purple-500 bg-zinc-800 p-5 shadow-[6px_6px_0px_0px_rgba(168,85,247,1)]">
      <div className="flex items-center gap-4">
        <div className="size-12 animate-pulse border-[3px] border-zinc-600 bg-zinc-700" />

        <div className="flex-1 space-y-3">
          <div className="h-5 w-28 animate-pulse bg-zinc-700" />
          <div className="h-4 w-full max-w-md animate-pulse bg-zinc-700" />
          <div className="h-3 w-2/3 max-w-xs animate-pulse bg-zinc-700" />
        </div>
      </div>
    </div>
  );
}
