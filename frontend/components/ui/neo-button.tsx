import type { ButtonHTMLAttributes } from "react";
import { cn } from "@/lib/cn";

type NeoButtonVariant = "purple" | "mint" | "white" | "danger";
type NeoButtonSize = "sm" | "md" | "lg";

const variantClasses: Record<NeoButtonVariant, string> = {
  purple:
    "border-white bg-purple-500 text-black shadow-[4px_4px_0px_0px_rgba(255,255,255,1)]",
  mint:
    "border-white bg-green-400 text-black shadow-[4px_4px_0px_0px_rgba(255,255,255,1)]",
  white:
    "border-purple-500 bg-white text-black shadow-[4px_4px_0px_0px_rgba(168,85,247,1)]",
  danger:
    "border-white bg-red-500 text-white shadow-[4px_4px_0px_0px_rgba(255,255,255,1)]",
};

const sizeClasses: Record<NeoButtonSize, string> = {
  sm: "min-h-10 px-3 py-2 text-xs",
  md: "min-h-12 px-5 py-3 text-sm",
  lg: "min-h-14 px-6 py-4 text-base",
};

export interface NeoButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: NeoButtonVariant;
  size?: NeoButtonSize;
}

export function NeoButton({
  variant = "purple",
  size = "md",
  className,
  type = "button",
  ...props
}: NeoButtonProps) {
  return (
    <button
      type={type}
      className={cn(
        "inline-flex items-center justify-center gap-2 border-[3px] font-black uppercase tracking-[0.12em] transition-all duration-75 hover:translate-x-[4px] hover:translate-y-[4px] hover:shadow-none focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-green-400 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:translate-x-0 disabled:hover:translate-y-0",
        variantClasses[variant],
        sizeClasses[size],
        className,
      )}
      {...props}
    />
  );
}