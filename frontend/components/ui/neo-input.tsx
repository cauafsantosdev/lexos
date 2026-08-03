import {
  forwardRef,
  type InputHTMLAttributes,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from "react";
import { cn } from "@/lib/cn";

const fieldClasses =
  "w-full rounded-none border-[3px] border-white bg-zinc-950 px-4 py-3 text-white shadow-[4px_4px_0px_0px_rgba(168,85,247,1)] outline-none transition-all duration-75 placeholder:text-zinc-500 focus:translate-x-[4px] focus:translate-y-[4px] focus:border-green-400 focus:shadow-none disabled:cursor-not-allowed disabled:opacity-50";

export const NeoInput = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function NeoInput({ className, ...props }, ref) {
    return <input ref={ref} className={cn(fieldClasses, className)} {...props} />;
  },
);

export const NeoTextarea = forwardRef<
  HTMLTextAreaElement,
  TextareaHTMLAttributes<HTMLTextAreaElement>
>(function NeoTextarea({ className, ...props }, ref) {
  return (
    <textarea
      ref={ref}
      className={cn(fieldClasses, "min-h-44 resize-y", className)}
      {...props}
    />
  );
});

export const NeoSelect = forwardRef<
  HTMLSelectElement,
  SelectHTMLAttributes<HTMLSelectElement>
>(function NeoSelect({ className, ...props }, ref) {
  return <select ref={ref} className={cn(fieldClasses, className)} {...props} />;
});
