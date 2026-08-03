import type { ReactNode } from "react";
import { NeoBadge } from "@/components/ui/neo-badge";

interface PageIntroProps {
  eyebrow: string;
  title: string;
  description: string;
  badge?: ReactNode;
}

export function PageIntro({ eyebrow, title, description, badge }: PageIntroProps) {
  return (
    <div className="mb-8 border-b-[3px] border-zinc-700 pb-8">
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <NeoBadge variant="purple">{eyebrow}</NeoBadge>
        {badge}
      </div>
      <h1 className="max-w-5xl text-4xl font-black uppercase leading-[0.95] tracking-[-0.05em] text-white md:text-6xl">
        {title}
      </h1>
      <p className="mt-5 max-w-3xl text-base font-medium leading-7 text-zinc-300 md:text-lg">
        {description}
      </p>
    </div>
  );
}
