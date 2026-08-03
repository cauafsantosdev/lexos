import type { HTMLAttributes } from "react";
import { cn } from "@/lib/cn";

type NeoBadgeVariant = "purple" | "mint" | "white" | "warning" | "danger";

const variantClasses: Record<NeoBadgeVariant, string> = {
  purple: "border-purple-500 bg-purple-500 text-black",
  mint: "border-green-400 bg-green-400 text-black",
  white: "border-white bg-white text-black",
  warning: "border-amber-300 bg-amber-300 text-black",
  danger: "border-red-400 bg-red-500 text-white",
};

export interface NeoBadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: NeoBadgeVariant;
}

export function NeoBadge({ variant = "white", className, ...props }: NeoBadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 border-2 px-2 py-1 font-mono text-[10px] font-black uppercase tracking-[0.16em]",
        variantClasses[variant],
        className,
      )}
      {...props}
    />
  );
}
