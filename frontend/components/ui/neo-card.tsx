import type { HTMLAttributes } from "react";
import { cn } from "@/lib/cn";

type NeoCardAccent = "purple" | "mint" | "white";

const accentClasses: Record<NeoCardAccent, string> = {
  purple:
    "border-purple-500 shadow-[6px_6px_0px_0px_rgba(168,85,247,1)]",
  mint: "border-green-400 shadow-[6px_6px_0px_0px_rgba(74,222,128,1)]",
  white: "border-white shadow-[6px_6px_0px_0px_rgba(255,255,255,1)]",
};

export interface NeoCardProps extends HTMLAttributes<HTMLDivElement> {
  accent?: NeoCardAccent;
  interactive?: boolean;
}

export function NeoCard({
  accent = "white",
  interactive = false,
  className,
  ...props
}: NeoCardProps) {
  return (
    <div
      className={cn(
        "border-[3px] bg-zinc-800 p-5",
        accentClasses[accent],
        interactive &&
          "transition-all duration-75 hover:translate-x-[6px] hover:translate-y-[6px] hover:shadow-none",
        className,
      )}
      {...props}
    />
  );
}
