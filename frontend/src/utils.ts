import { useQuery } from "@tanstack/react-query";
import { AuthResponse } from "./types";

const serverUrl = import.meta.env.MODE === "production" ? "" : "http://localhost:8080";

const fetchAuthSession = async (): Promise<AuthResponse> => {
    const response = await fetch(`${serverUrl}/auth/session`, {
        credentials: "include",
    });
    if (!response.ok) throw new Error("Failed to fetch auth session");
    return response.json();
};

export const useAuthQuery = () => {
    return useQuery({
        queryKey: ["authSession"],
        queryFn: fetchAuthSession,
        staleTime: 1000 * 60 * 5, // Cache for 5 minutes
        retry: false, // Don't retry failed requests
    });
};
