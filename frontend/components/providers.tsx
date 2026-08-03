"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { Toaster } from "sonner";

export function Providers({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: 1,
            refetchOnWindowFocus: false,
          },
          mutations: {
            retry: 0,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      <Toaster
        position="bottom-right"
        theme="dark"
        toastOptions={{
          classNames: {
            toast:
              "!rounded-none !border-[3px] !border-white !bg-zinc-800 !text-white !shadow-[4px_4px_0px_0px_rgba(168,85,247,1)]",
            title: "!font-bold",
            description: "!text-zinc-300",
            actionButton: "!rounded-none !bg-purple-500 !text-black",
            cancelButton: "!rounded-none !bg-zinc-700 !text-white",
          },
        }}
      />
    </QueryClientProvider>
  );
}
