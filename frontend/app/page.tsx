"use client";

import Link from "next/link";
import {
  Archive,
  ArrowRight,
  AudioLines,
  Bot,
  Boxes,
  Cpu,
  Braces,
  Database,
  FileText,
  ServerCog,
  Sparkles,
} from "lucide-react";
import { NeoBadge } from "@/components/ui/neo-badge";
import { NeoCard } from "@/components/ui/neo-card";

const services = [
  {
    href: "/scriber",
    name: "Scriber",
    description: "Upload audio and turn spoken words into a clear, readable transcript.",
    icon: AudioLines,
    accent: "purple" as const,
    tag: "Speech → Text",
  },
  {
    href: "/distiller",
    name: "Distiller",
    description: "Transform long documents or pasted text into focused, useful summaries.",
    icon: FileText,
    accent: "mint" as const,
    tag: "Document → Summary",
  },
  {
    href: "/gleaner",
    name: "Gleaner",
    description: "Index a document and ask questions grounded in its actual content.",
    icon: Bot,
    accent: "white" as const,
    tag: "Document → Answers",
  },
];

const architecture = [
  {
    label: "Next.js",
    detail: "User interface",
    icon: Braces,
    accent: "purple",
  },
  {
    label: "Go Gateway",
    detail: "Request orchestration",
    icon: ServerCog,
    accent: "white",
  },
  {
    label: "Redis",
    detail: "Task queue + status",
    icon: Database,
    accent: "mint",
  },
  {
    label: "Python Workers",
    detail: "Task routing",
    icon: Boxes,
    accent: "purple",
  },
  {
    label: "Inference Stack",
    detail: "Whisper, Qwen3, E5 Tokenizer",
    icon: Cpu,
    accent: "white",
  },
  {
    label: "MinIO",
    detail: "Artifact storage",
    icon: Archive,
    accent: "mint",
  },
] as const;

export default function DashboardPage() {
  return (
    <div className="space-y-12 pb-12">
      <section className="relative overflow-hidden border-[3px] border-white bg-zinc-800 p-6 shadow-[8px_8px_0px_0px_rgba(168,85,247,1)] md:p-10 lg:p-14">
        <div className="absolute -right-12 -top-12 hidden size-52 rotate-12 border-[3px] border-green-400 bg-purple-500 lg:block" />
        <div className="absolute right-20 top-24 hidden size-24 -rotate-6 border-[3px] border-white bg-green-400 lg:block" />

        <div className="relative max-w-5xl">
          <div className="mb-6 flex flex-wrap gap-3">
            <NeoBadge variant="mint">Document intelligence</NeoBadge>
            <NeoBadge variant="purple">AI workflows</NeoBadge>
            <NeoBadge variant="white">Built for focus</NeoBadge>
          </div>

          <h1 className="text-5xl font-black uppercase leading-[0.86] tracking-[-0.07em] text-white sm:text-6xl md:text-8xl xl:text-9xl">
            Process documents.
            <span className="block text-purple-400">Extract signal.</span>
          </h1>

          <p className="mt-8 max-w-3xl text-lg font-medium leading-8 text-zinc-300 md:text-xl">
            Lexos is a powerful AI document engine for asynchronous transcription,
            summarization, indexing, and retrieval-augmented generation.
          </p>

          <div className="mt-9 flex flex-wrap gap-4">
            <Link
              href="#services"
              className="inline-flex min-h-14 items-center gap-3 border-[3px] border-white bg-purple-500 px-6 py-4 font-black uppercase tracking-[0.12em] text-black shadow-[5px_5px_0px_0px_rgba(255,255,255,1)] transition-all duration-75 hover:translate-x-[5px] hover:translate-y-[5px] hover:shadow-none"
            >
              Explore Services
              <ArrowRight className="size-5" strokeWidth={3} />
            </Link>

            <span className="inline-flex min-h-14 items-center gap-3 border-[3px] border-green-400 bg-zinc-950 px-6 py-4 font-mono text-xs uppercase tracking-[0.14em] text-green-400">
              <span className="size-2 animate-pulse bg-green-400" />
              Ready to process
            </span>
          </div>
        </div>
      </section>

      <section>
        <div className="mb-6 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <NeoBadge variant="purple">System map</NeoBadge>
            <h2 className="mt-3 text-3xl font-black uppercase tracking-[-0.04em] md:text-5xl">
              Architecture diagram
            </h2>
          </div>

          <p className="max-w-xl font-mono text-xs leading-5 text-zinc-500">
            A clear view of how Lexos moves documents through the interface,
            orchestration layer, task queue, AI workers, inference engine and storage.
          </p>
        </div>

        <NeoCard accent="white" className="overflow-x-auto p-6 lg:p-8">
          <div className="grid min-w-[1180px] grid-cols-[1fr_auto_1fr_auto_1fr_auto_1fr_auto_1fr_auto_1fr] items-center gap-4">
            {architecture.map((node, index) => {
              const Icon = node.icon;

              return (
                <div key={node.label} className="contents">
                  <div
                    className={`border-[3px] bg-zinc-950 p-4 ${
                      node.accent === "purple"
                        ? "border-purple-500"
                        : node.accent === "mint"
                          ? "border-green-400"
                          : "border-white"
                    }`}
                  >
                    <Icon className="mb-6 size-7" strokeWidth={2.8} />
                    <p className="font-black uppercase tracking-[0.06em]">
                      {node.label}
                    </p>
                    <p className="mt-2 font-mono text-[11px] leading-5 text-zinc-500">
                      {node.detail}
                    </p>
                  </div>

                  {index < architecture.length - 1 && (
                    <div className="flex items-center" aria-hidden="true">
                      <span className="h-[3px] w-5 bg-white" />
                      <span className="size-0 border-y-[7px] border-l-[10px] border-y-transparent border-l-white" />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </NeoCard>
      </section>

      <section id="services">
        <div className="mb-6 flex items-center gap-3">
          <Sparkles className="size-7 text-green-400" />
          <h2 className="text-3xl font-black uppercase tracking-[-0.04em] md:text-5xl">
            Service modules
          </h2>
        </div>

        <div className="grid gap-7 xl:grid-cols-3">
          {services.map((service) => {
            const Icon = service.icon;

            return (
              <Link key={service.href} href={service.href} className="block">
                <NeoCard
                  accent={service.accent}
                  interactive
                  className="flex h-full min-h-72 flex-col p-7"
                >
                  <div className="flex items-start justify-between gap-4">
                    <span className="grid size-14 place-items-center border-[3px] border-white bg-zinc-950">
                      <Icon className="size-7" strokeWidth={2.8} />
                    </span>

                    <NeoBadge
                      variant={service.accent === "mint" ? "mint" : "purple"}
                    >
                      {service.tag}
                    </NeoBadge>
                  </div>

                  <h3 className="mt-10 text-4xl font-black uppercase tracking-[-0.05em]">
                    {service.name}
                  </h3>

                  <p className="mt-4 flex-1 leading-7 text-zinc-300">
                    {service.description}
                  </p>

                  <div className="mt-8 flex items-center gap-2 font-mono text-xs font-bold uppercase tracking-[0.14em] text-green-400">
                    Open service
                    <ArrowRight className="size-4" />
                  </div>
                </NeoCard>
              </Link>
            );
          })}
        </div>
      </section>
    </div>
  );
}
