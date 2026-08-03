"use client";

import { FileUp, X } from "lucide-react";
import { useId, useRef } from "react";
import { NeoButton } from "@/components/ui/neo-button";
import { cn } from "@/lib/cn";

interface FilePickerProps {
  label: string;
  hint: string;
  accept?: string;
  file: File | null;
  onChange: (file: File | null) => void;
  disabled?: boolean;
  accent?: "purple" | "mint";
}

export function FilePicker({
  label,
  hint,
  accept,
  file,
  onChange,
  disabled = false,
  accent = "purple",
}: FilePickerProps) {
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);

  return (
    <div
      className={cn(
        "border-[3px] border-dashed bg-zinc-950 p-5",
        accent === "purple" ? "border-purple-500" : "border-green-400",
      )}
    >
      <input
        ref={inputRef}
        id={inputId}
        type="file"
        accept={accept}
        disabled={disabled}
        className="sr-only"
        onChange={(event) => onChange(event.target.files?.[0] ?? null)}
      />

      {file ? (
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className="font-black uppercase tracking-[0.08em] text-white">Selected file</p>
            <p className="mt-1 truncate font-mono text-sm text-green-400">{file.name}</p>
            <p className="mt-1 font-mono text-xs text-zinc-500">
              {(file.size / 1024 / 1024).toFixed(2)} MB
            </p>
          </div>
          <NeoButton
            variant="danger"
            size="sm"
            disabled={disabled}
            onClick={() => {
              onChange(null);
              if (inputRef.current) {
                inputRef.current.value = "";
              }
            }}
          >
            <X className="size-4" /> Remove
          </NeoButton>
        </div>
      ) : (
        <label
          htmlFor={inputId}
          className={cn(
            "flex cursor-pointer flex-col items-center justify-center gap-3 py-8 text-center",
            disabled && "cursor-not-allowed opacity-50",
          )}
        >
          <span className="grid size-14 place-items-center border-[3px] border-white bg-zinc-800 shadow-[4px_4px_0px_0px_rgba(255,255,255,1)]">
            <FileUp className="size-7 text-purple-400" strokeWidth={2.8} />
          </span>
          <span className="font-black uppercase tracking-[0.1em] text-white">{label}</span>
          <span className="max-w-md font-mono text-xs leading-5 text-zinc-500">{hint}</span>
        </label>
      )}
    </div>
  );
}
