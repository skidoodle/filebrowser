import { useQuery } from "@tanstack/react-query";
import { auth } from "../api/auth";

/** Resolves the caller's capabilities (admin, insecure, initialized). */
export function useMe() {
  return useQuery({ queryKey: ["me"], queryFn: auth.me, staleTime: 60_000 });
}
