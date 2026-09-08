import { QueryClient } from "@tanstack/react-query";

/** Shared instance so non-React modules (e.g. the upload store) can
 * invalidate queries when background work finishes. */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 5_000,
    },
  },
});
