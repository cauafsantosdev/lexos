"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { AudioLines, Bot, FileText, Github, Sparkles } from "lucide-react";
import { cn } from "@/lib/cn";

const navigation = [
  { href: "/scriber", label: "Scriber", icon: AudioLines },
  { href: "/distiller", label: "Distiller", icon: FileText },
  { href: "/gleaner", label: "Gleaner", icon: Bot },
] as const;

const githubUrl = process.env.NEXT_PUBLIC_GITHUB_URL ?? "https://github.com/cauafsantosdev/lexos";

function GithubIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="currentColor"
      {...props}
    >
      <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
    </svg>
  );
}

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="fixed inset-y-0 left-0 z-40 flex w-20 flex-col border-r-[3px] border-white bg-zinc-950 p-3 lg:w-72 lg:p-5">
      <Link
        href="/"
        aria-label="Lexos home"
        className="mb-8 flex items-center gap-3 border-[3px] border-purple-500 bg-zinc-900 p-3 shadow-[4px_4px_0px_0px_rgba(168,85,247,1)] transition-all duration-75 hover:translate-x-[4px] hover:translate-y-[4px] hover:shadow-none"
      >
        <span className="grid size-9 shrink-0 place-items-center bg-purple-500 text-black">
          <Sparkles className="size-5" strokeWidth={3} />
        </span>

        <span className="hidden lg:block">
          <span className="block text-xl font-black uppercase tracking-[-0.04em]">
            Lexos
          </span>
          <span className="block font-mono text-[10px] uppercase tracking-[0.2em] text-green-400">
            AI Document Engine
          </span>
        </span>
      </Link>

      <nav className="flex flex-1 flex-col gap-3" aria-label="Primary navigation">
        {navigation.map((item) => {
          const active = pathname.startsWith(item.href);
          const Icon = item.icon;

          return (
            <Link
              key={item.href}
              href={item.href}
              aria-current={active ? "page" : undefined}
              title={item.label}
              className={cn(
                "flex min-h-14 items-center justify-center gap-3 border-[3px] px-3 py-3 font-black uppercase tracking-[0.12em] transition-all duration-75 hover:translate-x-[4px] hover:translate-y-[4px] hover:shadow-none lg:justify-start",
                active
                  ? "border-white bg-green-400 text-black shadow-[4px_4px_0px_0px_rgba(255,255,255,1)]"
                  : "border-zinc-600 bg-zinc-900 text-zinc-200 shadow-[4px_4px_0px_0px_rgba(168,85,247,1)] hover:border-purple-500",
              )}
            >
              <Icon className="size-5 shrink-0" strokeWidth={2.8} />
              <span className="hidden text-sm lg:inline">{item.label}</span>
            </Link>
          );
        })}
      </nav>

      <a
        href={githubUrl}
        target="_blank"
        rel="noreferrer"
        aria-label="Open Lexos on GitHub"
        title="GitHub"
        className="flex min-h-14 items-center justify-center gap-3 border-[3px] border-white bg-zinc-900 px-3 py-3 font-black uppercase tracking-[0.12em] text-white shadow-[4px_4px_0px_0px_rgba(74,222,128,1)] transition-all duration-75 hover:translate-x-[4px] hover:translate-y-[4px] hover:bg-green-400 hover:text-black hover:shadow-none lg:justify-start"
      >
        <GithubIcon className="size-5 shrink-0" />
        <span className="hidden text-sm lg:inline">GitHub</span>
      </a>
    </aside>
  );
}